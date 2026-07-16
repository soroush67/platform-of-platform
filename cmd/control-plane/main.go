// The Control Plane binary - walking skeleton per docs/architecture/21-deployment.md
// §4's bootstrap sequence: connect to Postgres, migrate on startup, serve HTTP.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/cockroachdb"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
	grpcserver "google.golang.org/grpc"

	audithttp "platform-of-platform/internal/audit/adapters/http"
	auditpg "platform-of-platform/internal/audit/adapters/postgres"
	auditapp "platform-of-platform/internal/audit/application"
	executiongrpc "platform-of-platform/internal/execution/adapters/grpc"
	executionpb "platform-of-platform/internal/execution/adapters/grpc/proto"
	executionhttp "platform-of-platform/internal/execution/adapters/http"
	executionpg "platform-of-platform/internal/execution/adapters/postgres"
	executionapp "platform-of-platform/internal/execution/application"
	fleethttp "platform-of-platform/internal/fleet/adapters/http"
	fleetpg "platform-of-platform/internal/fleet/adapters/postgres"
	fleetredisstream "platform-of-platform/internal/fleet/adapters/redisstream"
	fleetssh "platform-of-platform/internal/fleet/adapters/ssh"
	fleetapp "platform-of-platform/internal/fleet/application"
	identityhttp "platform-of-platform/internal/identity/adapters/http"
	identitypg "platform-of-platform/internal/identity/adapters/postgres"
	identityapp "platform-of-platform/internal/identity/application"
	identitydomain "platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/config"
	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/platform/idempotency"
	"platform-of-platform/internal/platform/mtls"
	"platform-of-platform/internal/platform/outbox"
	"platform-of-platform/internal/platform/ratelimit"
	"platform-of-platform/internal/platform/tracing"
	rbachttp "platform-of-platform/internal/rbac/adapters/http"
	rbacpg "platform-of-platform/internal/rbac/adapters/postgres"
	rbacapp "platform-of-platform/internal/rbac/application"
	rbacdomain "platform-of-platform/internal/rbac/domain"
	secretshttp "platform-of-platform/internal/secrets/adapters/http"
	secretspg "platform-of-platform/internal/secrets/adapters/postgres"
	secretsvault "platform-of-platform/internal/secrets/adapters/vault"
	secretsapp "platform-of-platform/internal/secrets/application"
	secretsdomain "platform-of-platform/internal/secrets/domain"
	tenancyhttp "platform-of-platform/internal/tenancy/adapters/http"
	tenancypg "platform-of-platform/internal/tenancy/adapters/postgres"
	tenancyapp "platform-of-platform/internal/tenancy/application"
	variableshttp "platform-of-platform/internal/variables/adapters/http"
	variablespg "platform-of-platform/internal/variables/adapters/postgres"
	variablesapp "platform-of-platform/internal/variables/application"
	workspacehttp "platform-of-platform/internal/workspace/adapters/http"
	workspacepg "platform-of-platform/internal/workspace/adapters/postgres"
	workspaceapp "platform-of-platform/internal/workspace/application"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	if err := runMigrations(cfg, logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}

	tracingShutdown, err := tracing.Setup(context.Background(), "control-plane")
	if err != nil {
		logger.Error("tracing setup failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tracingShutdown(context.Background()); err != nil {
			logger.Error("tracing shutdown failed", "error", err)
		}
	}()

	pool, err := pgxpool.New(context.Background(), cfg.AppDatabaseURL)
	if err != nil {
		logger.Error("db pool init failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// rootPool - the same role migrations use (cfg.DatabaseURL, not
	// cfg.AppDatabaseURL) - exists at runtime for exactly one reason:
	// idempotency.Reaper's cleanup sweep is a genuine cross-org DELETE
	// against a table (idempotency_keys) that, unlike outbox_events, DOES
	// have FORCE ROW LEVEL SECURITY, so platform_app has no way to see
	// across every org's rows in one query (see Reaper's own doc comment
	// for the full reasoning, and Purge's own comment for the same class
	// of bug found for real when this was missed once already).
	rootPool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("root db pool init failed", "error", err)
		os.Exit(1)
	}
	defer rootPool.Close()

	// redisClient backs the Worker Registry's multi-instance HA routing
	// (internal/execution/adapters/grpc's own doc comment) - shared,
	// cross-replica runID/Worker-ownership state, the "cache/
	// coordination, never system-of-record" role docs/architecture/
	// 05-database.md §5 reserves for Redis. instanceID identifies this
	// replica: os.Hostname() returns Docker's own real, random
	// per-container ID by default, a genuine unique identity with no
	// extra coordination needed to obtain one.
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer redisClient.Close()
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Error("redis connect failed", "error", err)
		os.Exit(1)
	}
	instanceID, err := os.Hostname()
	if err != nil {
		logger.Error("failed to determine this instance's own hostname", "error", err)
		os.Exit(1)
	}

	roleRepo := rbacpg.NewRoleRepository(pool)
	if err := roleRepo.SeedBuiltinRoles(context.Background()); err != nil {
		logger.Error("role seeding failed", "error", err)
		os.Exit(1)
	}
	logger.Info("builtin roles seeded")

	// Manual wiring, in one place - docs/architecture/18-backend-structure.md §5's
	// "no DI framework" decision: every dependency is greppable from here.
	roleBindingRepo := rbacpg.NewRoleBindingRepository(pool)
	// Constructed early (not with the rest of Identity's wiring below) -
	// CreateRoleBindingService needs it as a ServiceAccountChecker to
	// validate subject_type='service_account' bindings.
	serviceAccountRepo := identitypg.NewServiceAccountRepository(pool)

	orgRepo := tenancypg.NewOrganizationRepository(pool)
	membershipRepo := tenancypg.NewMembershipRepository(pool)
	projectRepo := tenancypg.NewProjectRepository(pool)
	teamRepo := tenancypg.NewTeamRepository(pool)
	createOrgService := tenancyapp.NewCreateOrganizationService(orgRepo, membershipRepo, roleBindingRepo)
	getOrgService := tenancyapp.NewGetOrganizationService(orgRepo, membershipRepo)
	// rootMembershipRepo - a deliberately separate, root-connection-backed
	// repository used ONLY for ListMyOrganizationsService's genuine
	// cross-org read (see application.RootMembershipRepository's own doc
	// comment for the full RLS reasoning).
	rootMembershipRepo := tenancypg.NewRootMembershipRepository(rootPool)
	listMyOrganizationsService := tenancyapp.NewListMyOrganizationsService(rootMembershipRepo)
	addMemberService := tenancyapp.NewAddMemberService(membershipRepo, roleBindingRepo, roleBindingRepo)
	changeMemberRoleService := tenancyapp.NewChangeMemberRoleService(membershipRepo, roleBindingRepo, roleBindingRepo)
	createProjectService := tenancyapp.NewCreateProjectService(projectRepo, membershipRepo, roleBindingRepo, orgRepo)
	listProjectsService := tenancyapp.NewListProjectsService(projectRepo, membershipRepo)
	getProjectService := tenancyapp.NewGetProjectService(projectRepo, membershipRepo)
	createTeamService := tenancyapp.NewCreateTeamService(teamRepo, membershipRepo, roleBindingRepo)
	addTeamMemberService := tenancyapp.NewAddTeamMemberService(teamRepo, membershipRepo, roleBindingRepo)
	removeTeamMemberService := tenancyapp.NewRemoveTeamMemberService(teamRepo, membershipRepo, roleBindingRepo)
	archiveOrganizationService := tenancyapp.NewArchiveOrganizationService(orgRepo, orgRepo, membershipRepo, roleBindingRepo)

	// RBAC's own first-class endpoints (docs/architecture/13-module-
	// identity-rbac-tenancy.md §3) - custom roles + generic role-bindings,
	// previously entirely unbuilt (every *other* context only ever used
	// roleBindingRepo as a cross-context port, never RBAC's own surface).
	createRoleService := rbacapp.NewCreateRoleService(roleRepo, membershipRepo, roleBindingRepo)
	listRolesService := rbacapp.NewListRolesService(roleRepo, membershipRepo)
	listRoleBindingsService := rbacapp.NewListRoleBindingsService(roleBindingRepo, membershipRepo)

	environmentRepo := workspacepg.NewEnvironmentRepository(pool)
	workspaceRepo := workspacepg.NewWorkspaceRepository(pool)
	// CreateRoleBindingService needs workspaceRepo (validates
	// scope.id for scope.type=workspace bindings) - wired here, after
	// workspaceRepo exists, not up with the other RBAC services above.
	createRoleBindingService := rbacapp.NewCreateRoleBindingService(roleRepo, roleBindingRepo, membershipRepo, roleBindingRepo, projectRepo, workspaceRepo, teamRepo, serviceAccountRepo)
	createEnvironmentService := workspaceapp.NewCreateEnvironmentService(environmentRepo, membershipRepo, roleBindingRepo, projectRepo)
	listEnvironmentsService := workspaceapp.NewListEnvironmentsService(environmentRepo, membershipRepo, projectRepo)
	getEnvironmentService := workspaceapp.NewGetEnvironmentService(environmentRepo, membershipRepo, projectRepo)
	createWorkspaceService := workspaceapp.NewCreateWorkspaceService(workspaceRepo, environmentRepo, membershipRepo, roleBindingRepo, projectRepo, orgRepo)
	listWorkspacesService := workspaceapp.NewListWorkspacesService(workspaceRepo, membershipRepo, projectRepo)
	getWorkspaceService := workspaceapp.NewGetWorkspaceService(workspaceRepo, membershipRepo, projectRepo)

	// Worker registry + gRPC server (docs/architecture/17-workers.md §1) -
	// created before the Execution services below since CancelRunService
	// now needs it too (registry.Dispatch/CancelJob structurally satisfy
	// Execution's own WorkerDispatcher/WorkerCanceler ports, same "one
	// concrete type satisfies several ports" pattern already used for
	// roleBindingRepo/workspaceRepo).
	workerRegistry := executiongrpc.NewRegistry(instanceID, redisClient, logger)

	runRepo := executionpg.NewRunRepository(pool)
	triggerRunService := executionapp.NewTriggerRunService(runRepo, workspaceRepo, workspaceRepo, roleBindingRepo, orgRepo)
	cancelRunService := executionapp.NewCancelRunService(runRepo, workspaceRepo, roleBindingRepo, workerRegistry)
	listRunsService := executionapp.NewListRunsService(runRepo, membershipRepo, workspaceRepo)
	getRunService := executionapp.NewGetRunService(runRepo, membershipRepo, workspaceRepo)
	workerReportService := executionapp.NewWorkerReportService(runRepo, workspaceRepo, workerRegistry)
	staleRunReaper := executionapp.NewStaleRunReaperService(runRepo, workspaceRepo, workerRegistry, cfg.RunStaleAfter, cfg.RunReaperInterval, logger)
	purgeReaper := tenancyapp.NewPurgeReaperService(orgRepo, cfg.OrgPurgeAfter, cfg.OrgPurgeReaperInterval, logger)

	// Secrets/Vault integration (docs/architecture/11-module-secrets-
	// state.md §1) - secretMountRepo/vaultClient are shared by all four
	// Secrets services below; resolveSecretService is also handed
	// straight to Variables as its SecretResolver port (its ResolveValue
	// method already matches that port's signature with zero adapter
	// glue, same "one concrete type satisfies several ports" pattern as
	// roleBindingRepo/workspaceRepo above).
	secretMountRepo := secretspg.NewSecretMountRepository(pool)
	vaultClient := secretsvault.NewClient()
	createSecretMountService := secretsapp.NewCreateSecretMountService(secretMountRepo, membershipRepo, roleBindingRepo, cfg.SecretsMasterKey)
	listSecretMountsService := secretsapp.NewListSecretMountsService(secretMountRepo, membershipRepo)
	testConnectionService := secretsapp.NewTestConnectionService(secretMountRepo, membershipRepo, roleBindingRepo, vaultClient, cfg.SecretsMasterKey)
	resolveSecretService := secretsapp.NewResolveSecretService(secretMountRepo, vaultClient, cfg.SecretsMasterKey)

	// secretMountCheckerFunc bridges Variables' own SecretMountChecker
	// port to Secrets' repository - Variables never imports secrets/
	// domain directly (docs/architecture/18-backend-structure.md §3's
	// dependency-inversion rule), so this closure (only main.go is
	// allowed to see both contexts) is what translates
	// secretsdomain.ErrSecretMountNotFound into the plain (false, nil)
	// SecretMountChecker's own contract expects.
	secretMountCheckerFunc := variablesapp.SecretMountCheckerFunc(func(ctx context.Context, organizationID, mountID string) (bool, error) {
		if _, err := secretMountRepo.GetByID(ctx, organizationID, mountID); err != nil {
			if errors.Is(err, secretsdomain.ErrSecretMountNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})

	variableRepo := variablespg.NewVariableRepository(pool)
	createVariableService := variablesapp.NewCreateVariableService(variableRepo, membershipRepo, projectRepo, environmentRepo, workspaceRepo, roleBindingRepo, orgRepo, secretMountCheckerFunc)
	listVariablesService := variablesapp.NewListVariablesService(variableRepo, membershipRepo)
	resolveVariableService := variablesapp.NewResolveVariableService(variableRepo, membershipRepo, workspaceRepo, resolveSecretService)
	updateVariableService := variablesapp.NewUpdateVariableService(variableRepo, membershipRepo, roleBindingRepo)
	deleteVariableService := variablesapp.NewDeleteVariableService(variableRepo, membershipRepo, roleBindingRepo)

	// Fleet (the Fleet plan, /home/soroush/.claude/plans/elegant-tickling-
	// cosmos.md) - ports compose-platform's Machine/Network/Volume/
	// ComposeFile/Variable/Operation feature set natively into this
	// codebase. membershipRepo/roleBindingRepo/resolveSecretService are
	// reused directly (structural interface satisfaction - the same "one
	// concrete type satisfies several ports" pattern already used for
	// roleBindingRepo/workspaceRepo above); secretMountCheckerFunc too -
	// its SecretMountExists method already matches fleetapp.
	// SecretMountChecker's own identical method signature, so it
	// satisfies that interface with zero adapter glue despite being typed
	// as variablesapp.SecretMountCheckerFunc.
	fleetNetworkRepo := fleetpg.NewNetworkRepository(pool)
	fleetVolumeRepo := fleetpg.NewVolumeRepository(pool)
	fleetMachineRepo := fleetpg.NewMachineRepository(pool)
	fleetComposeFileRepo := fleetpg.NewComposeFileRepository(pool)
	fleetAttachmentRepo := fleetpg.NewAttachmentRepository(pool)
	fleetVariableRepo := fleetpg.NewVariableRepository(pool)
	fleetOperationRepo := fleetpg.NewOperationRepository(pool)
	// fleetOperationScanner - root-pool-backed cross-org scan, same
	// exception StaleRunReaperService/idempotency.Reaper already
	// establish for tenant-facing tables the app pool's RLS can't see
	// across every org in one query.
	fleetOperationScanner := fleetpg.NewOperationScanner(rootPool)
	fleetSSHClient := fleetssh.NewClient()
	fleetLogPublisher := fleetredisstream.NewPublisher(redisClient)

	createNetworkService := fleetapp.NewCreateNetworkService(fleetNetworkRepo, membershipRepo, roleBindingRepo)
	listNetworksService := fleetapp.NewListNetworksService(fleetNetworkRepo, membershipRepo)
	deleteNetworkService := fleetapp.NewDeleteNetworkService(fleetNetworkRepo, membershipRepo, roleBindingRepo)
	createVolumeService := fleetapp.NewCreateVolumeService(fleetVolumeRepo, membershipRepo, roleBindingRepo)
	listVolumesService := fleetapp.NewListVolumesService(fleetVolumeRepo, membershipRepo)
	deleteVolumeService := fleetapp.NewDeleteVolumeService(fleetVolumeRepo, membershipRepo, roleBindingRepo)
	createMachineService := fleetapp.NewCreateMachineService(fleetMachineRepo, membershipRepo, roleBindingRepo, secretMountCheckerFunc)
	listMachinesService := fleetapp.NewListMachinesService(fleetMachineRepo, membershipRepo)
	getMachineService := fleetapp.NewGetMachineService(fleetMachineRepo, membershipRepo)
	updateMachineService := fleetapp.NewUpdateMachineService(fleetMachineRepo, membershipRepo, roleBindingRepo, secretMountCheckerFunc)
	archiveMachineService := fleetapp.NewArchiveMachineService(fleetMachineRepo, membershipRepo, roleBindingRepo)
	testMachineConnectionService := fleetapp.NewTestMachineConnectionService(membershipRepo, roleBindingRepo, resolveSecretService, fleetSSHClient)
	checkMachineConnectionService := fleetapp.NewCheckMachineConnectionService(fleetMachineRepo, membershipRepo, roleBindingRepo, resolveSecretService, fleetSSHClient)
	createComposeFileService := fleetapp.NewCreateComposeFileService(fleetComposeFileRepo, membershipRepo, roleBindingRepo)
	listComposeFilesService := fleetapp.NewListComposeFilesService(fleetComposeFileRepo, membershipRepo)
	getComposeFileService := fleetapp.NewGetComposeFileService(fleetComposeFileRepo, membershipRepo)
	updateComposeFileContentService := fleetapp.NewUpdateComposeFileContentService(fleetComposeFileRepo, membershipRepo, roleBindingRepo)
	fleetAttachmentService := fleetapp.NewAttachmentService(fleetAttachmentRepo, membershipRepo, roleBindingRepo)
	createFleetVariableService := fleetapp.NewCreateVariableService(fleetVariableRepo, membershipRepo, roleBindingRepo, secretMountCheckerFunc)
	listFleetVariablesService := fleetapp.NewListVariablesService(fleetVariableRepo, membershipRepo)
	updateFleetVariableService := fleetapp.NewUpdateVariableService(fleetVariableRepo, membershipRepo, roleBindingRepo, secretMountCheckerFunc)
	deleteFleetVariableService := fleetapp.NewDeleteVariableService(fleetVariableRepo, membershipRepo, roleBindingRepo)
	triggerOperationService := fleetapp.NewTriggerOperationService(fleetOperationRepo, fleetComposeFileRepo, fleetMachineRepo, membershipRepo, roleBindingRepo)
	listOperationsService := fleetapp.NewListOperationsService(fleetOperationRepo, membershipRepo)
	getOperationService := fleetapp.NewGetOperationService(fleetOperationRepo, membershipRepo)

	// fleetDeployExecutor - Fleet's own Runnable (see decision #3 in the
	// Fleet plan: in-process, not the gRPC Worker/Job/Run model), started
	// as one more g.Go(...) line in the existing errgroup below. 2s poll
	// interval matches outbox.Relay's own.
	fleetDeployExecutor := fleetapp.NewDeployExecutor(
		fleetOperationScanner, fleetOperationRepo, fleetMachineRepo, fleetComposeFileRepo, fleetVariableRepo,
		fleetAttachmentRepo, resolveSecretService, fleetSSHClient, fleetLogPublisher, 2*time.Second, logger,
	)

	runDispatchService := executionapp.NewRunDispatchService(runRepo, workspaceRepo, resolveVariableService, workerRegistry, workspaceRepo)
	grpcWorkerServer := executiongrpc.NewServer(workerRegistry, workerReportService.HandleReport)

	auditRepo := auditpg.NewAuditEntryRepository(pool)
	recordEntryService := auditapp.NewRecordEntryService(auditRepo)
	listAuditEntriesService := auditapp.NewListAuditEntriesService(auditRepo, roleBindingRepo)

	// One combined outbox.Handler fanning out to both real consumers of
	// this codebase's events - Audit (records everything) and the Run
	// Dispatcher (acts only on RunQueued) - since outbox_events has a
	// single published_at flag, not a per-consumer delivery table
	// (internal/platform/outbox/outbox.go's own scope). Both handlers
	// are safe to re-run on a redelivery: Audit via source_event_id's
	// ON CONFLICT DO NOTHING, Run Dispatch via TryStartApplying's atomic
	// compare-and-swap.
	combinedHandler := func(ctx context.Context, event outbox.Event) error {
		if err := recordEntryService.HandleEvent(ctx, event); err != nil {
			return err
		}
		return runDispatchService.HandleEvent(ctx, event)
	}
	relay := outbox.NewRelay(pool, combinedHandler, 2*time.Second, logger)

	userRepo := identitypg.NewUserRepository(pool)
	refreshTokenRepo := identitypg.NewRefreshTokenRepository(pool)
	passwordResetTokenRepo := identitypg.NewPasswordResetTokenRepository(pool)
	apiKeyRepo := identitypg.NewAPIKeyRepository(pool)
	createUserService := identityapp.NewCreateUserService(userRepo)
	getOwnUserService := identityapp.NewGetOwnUserService(userRepo)
	authenticateService := identityapp.NewAuthenticateService(userRepo)
	refreshTokenService := identityapp.NewRefreshTokenService(refreshTokenRepo, userRepo)
	passwordResetService := identityapp.NewPasswordResetService(passwordResetTokenRepo, userRepo, logger)
	createServiceAccountService := identityapp.NewCreateServiceAccountService(serviceAccountRepo, membershipRepo, roleBindingRepo)
	// scopeValidatorFunc closes over rbac/domain.AllPermissions - the
	// dependency-inversion shape identity/application.ScopeValidator's
	// own comment describes: Identity can't import rbac/domain directly,
	// so main.go (which is allowed to see both) bridges the two.
	createAPIKeyService := identityapp.NewCreateAPIKeyService(apiKeyRepo, serviceAccountRepo, membershipRepo, roleBindingRepo, identityapp.ScopeValidatorFunc(func(scope string) bool {
		return rbacdomain.AllPermissions[rbacdomain.Permission(scope)]
	}))
	revokeAPIKeyService := identityapp.NewRevokeAPIKeyService(apiKeyRepo, membershipRepo, roleBindingRepo)
	// listMembersService's UserReader/RoleReader ports are satisfied
	// structurally by userRepo/roleBindingRepo (internal/tenancy/
	// application/ports.go's own dependency-inversion pattern - Tenancy
	// declares the interfaces it needs, Identity/RBAC's real adapters
	// happen to implement them, no explicit wiring beyond passing them
	// in here).
	listMembersService := tenancyapp.NewListMembersService(membershipRepo, userRepo, roleBindingRepo)

	// The real API-key authentication path (docs/architecture/13-module-
	// identity-rbac-tenancy.md §2) - httpserver.RequireAuth calls this for
	// any bearer token that isn't JWT-shaped. Real validation (expiry,
	// revocation - APIKey.Valid()), not just "does a row with this hash
	// exist" - and a real best-effort last_used_at touch on every
	// successful auth, the same bookkeeping field the doc names.
	apiKeyResolver := httpserver.APIKeyResolver(func(ctx context.Context, plaintextKey string) (string, []string, error) {
		key, err := apiKeyRepo.GetByHash(ctx, auth.HashOpaqueToken(plaintextKey))
		if err != nil {
			return "", nil, err
		}
		if !key.Valid() {
			return "", nil, identitydomain.ErrAPIKeyInvalid
		}
		_ = apiKeyRepo.TouchLastUsed(ctx, key.ID)
		// key.Scopes flows into principal.WithScopes (httpserver.RequireAuth)
		// and from there into RoleBindingRepository.HasPermissionAtScope's
		// own real intersection - previously computed and returned by the
		// API but never actually enforced anywhere.
		return key.OwnerID, key.Scopes, nil
	})

	// Rate limiting (docs/architecture's own deferred cross-cutting gap,
	// built for real now) - loginLimiter is the narrow, high-value
	// defense (5 attempts/5min per *username*, brute-force/credential-
	// stuffing specific); generalLimiter (100 req/min per client IP)
	// wraps the entire mux below, a general abuse backstop every request
	// pays regardless of which endpoint or whether it's authenticated
	// yet. Both in-memory, single-instance-only - see
	// internal/platform/ratelimit's own package comment on why.
	loginLimiter := ratelimit.New(5, 5*time.Minute)
	generalLimiter := ratelimit.New(100, time.Minute)
	rateLimitGC := ratelimit.NewGCLoop(time.Minute, loginLimiter, generalLimiter)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(pool))
	mux.HandleFunc("POST /api/v1/users", httpserver.RequireAuthOrFirstUserBootstrap(cfg.JWTSigningKey, apiKeyResolver, userRepo.Count, identityhttp.CreateUserHandler(createUserService)))
	mux.HandleFunc("GET /api/v1/users/me", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.GetOwnUserHandler(getOwnUserService)))
	mux.HandleFunc("POST /api/v1/auth/login", identityhttp.LoginHandler(authenticateService, refreshTokenService, loginLimiter, cfg.JWTSigningKey))
	mux.HandleFunc("POST /api/v1/auth/refresh", identityhttp.RefreshTokenHandler(refreshTokenService, cfg.JWTSigningKey))
	mux.HandleFunc("POST /api/v1/auth/password-reset/request", identityhttp.RequestPasswordResetHandler(passwordResetService))
	mux.HandleFunc("POST /api/v1/auth/password-reset/confirm", identityhttp.ConfirmPasswordResetHandler(passwordResetService))
	mux.HandleFunc("GET /api/v1/orgs", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.ListMyOrganizationsHandler(listMyOrganizationsService)))
	mux.HandleFunc("POST /api/v1/orgs", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.CreateOrganizationHandler(createOrgService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.GetOrganizationHandler(getOrgService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/members", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.AddMemberHandler(addMemberService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/members", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.ListMembersHandler(listMembersService)))
	mux.HandleFunc("PUT /api/v1/orgs/{id}/members/{userID}/role", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.ChangeMemberRoleHandler(changeMemberRoleService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.ArchiveOrganizationHandler(archiveOrganizationService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/teams", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.CreateTeamHandler(createTeamService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/teams/{team}/members", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.AddTeamMemberHandler(addTeamMemberService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/teams/{team}/members/{user_id}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.RemoveTeamMemberHandler(removeTeamMemberService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/roles", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, rbachttp.CreateRoleHandler(createRoleService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/roles", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, rbachttp.ListRolesHandler(listRolesService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/role-bindings", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, rbachttp.CreateRoleBindingHandler(createRoleBindingService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/role-bindings", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, rbachttp.ListRoleBindingsHandler(listRoleBindingsService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/projects", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.CreateProjectHandler(createProjectService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.ListProjectsHandler(listProjectsService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.GetProjectHandler(getProjectService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/projects/{projectID}/environments", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, workspacehttp.CreateEnvironmentHandler(createEnvironmentService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/environments", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, workspacehttp.ListEnvironmentsHandler(listEnvironmentsService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/environments/{envID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, workspacehttp.GetEnvironmentHandler(getEnvironmentService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/projects/{projectID}/workspaces", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, workspacehttp.CreateWorkspaceHandler(createWorkspaceService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/workspaces", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, workspacehttp.ListWorkspacesHandler(listWorkspacesService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/workspaces/{workspaceID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, workspacehttp.GetWorkspaceHandler(getWorkspaceService)))
	idempotencyStore := idempotency.NewStore(pool)
	idempotencyReaper := idempotency.NewReaper(rootPool, cfg.IdempotencyReaperInterval, logger)
	mux.HandleFunc("POST /api/v1/orgs/{id}/projects/{projectID}/workspaces/{workspaceID}/runs", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, idempotency.Middleware(idempotencyStore, executionhttp.TriggerRunHandler(triggerRunService))))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/workspaces/{workspaceID}/runs", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, executionhttp.ListRunsHandler(listRunsService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/workspaces/{workspaceID}/runs/{runID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, executionhttp.GetRunHandler(getRunService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/projects/{projectID}/workspaces/{workspaceID}/runs/{runID}/cancel", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, executionhttp.CancelRunHandler(cancelRunService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/variables", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, variableshttp.CreateVariableHandler(createVariableService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/variables", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, variableshttp.ListVariablesHandler(listVariablesService)))
	mux.HandleFunc("PUT /api/v1/orgs/{id}/variables/{variableID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, variableshttp.UpdateVariableHandler(updateVariableService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/variables/{variableID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, variableshttp.DeleteVariableHandler(deleteVariableService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/projects/{projectID}/workspaces/{workspaceID}/variables/resolve", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, variableshttp.ResolveVariableHandler(resolveVariableService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/secret-mounts", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, secretshttp.CreateSecretMountHandler(createSecretMountService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/secret-mounts", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, secretshttp.ListSecretMountsHandler(listSecretMountsService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/secret-mounts/{mount}/test-connection", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, secretshttp.TestConnectionHandler(testConnectionService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/audit-log", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, audithttp.ListAuditLogHandler(listAuditEntriesService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/service-accounts", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.CreateServiceAccountHandler(createServiceAccountService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/service-accounts/{sa}/api-keys", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.CreateAPIKeyHandler(createAPIKeyService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/service-accounts/{sa}/api-keys/{key}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.RevokeAPIKeyHandler(revokeAPIKeyService)))

	// Fleet's own HTTP surface (docs/architecture's newest bounded
	// context) - flat under /orgs/{id}/, matching the dominant existing
	// pattern (secret-mounts/variables/roles/teams are all flat siblings,
	// no /fleet/ prefix). "test-connection" is registered as a literal
	// sibling of the {machineID} wildcard route - net/http's own ServeMux
	// (Go 1.22+) prefers a matching literal segment over a wildcard, so
	// there's no route-ordering hazard here.
	mux.HandleFunc("POST /api/v1/orgs/{id}/machines", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.CreateMachineHandler(createMachineService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/machines", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListMachinesHandler(listMachinesService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/machines/test-connection", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.TestMachineConnectionHandler(testMachineConnectionService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/machines/{machineID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.GetMachineHandler(getMachineService)))
	mux.HandleFunc("PATCH /api/v1/orgs/{id}/machines/{machineID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.UpdateMachineHandler(updateMachineService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/machines/{machineID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ArchiveMachineHandler(archiveMachineService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/machines/{machineID}/check-connection", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.CheckMachineConnectionHandler(checkMachineConnectionService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/networks", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.CreateNetworkHandler(createNetworkService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/networks", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListNetworksHandler(listNetworksService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/networks/{networkID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.DeleteNetworkHandler(deleteNetworkService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/volumes", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.CreateVolumeHandler(createVolumeService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/volumes", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListVolumesHandler(listVolumesService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/volumes/{volumeID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.DeleteVolumeHandler(deleteVolumeService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/compose-files", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.CreateComposeFileHandler(createComposeFileService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/compose-files", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListComposeFilesHandler(listComposeFilesService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/compose-files/{composeFileID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.GetComposeFileHandler(getComposeFileService)))
	mux.HandleFunc("PUT /api/v1/orgs/{id}/compose-files/{composeFileID}/content", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.UpdateComposeFileContentHandler(updateComposeFileContentService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/compose-files/{composeFileID}/networks", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.AttachNetworkHandler(fleetAttachmentService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/compose-files/{composeFileID}/networks", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListComposeFileNetworksHandler(fleetAttachmentService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/compose-files/{composeFileID}/networks/{networkID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.DetachNetworkHandler(fleetAttachmentService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/compose-files/{composeFileID}/volumes", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.AttachVolumeHandler(fleetAttachmentService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/compose-files/{composeFileID}/volumes", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListComposeFileVolumesHandler(fleetAttachmentService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/compose-files/{composeFileID}/volumes/{volumeID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.DetachVolumeHandler(fleetAttachmentService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/compose-files/{composeFileID}/variables", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.CreateVariableHandler(createFleetVariableService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/compose-files/{composeFileID}/variables", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListVariablesHandler(listFleetVariablesService)))
	mux.HandleFunc("PUT /api/v1/orgs/{id}/compose-files/{composeFileID}/variables/{variableID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.UpdateVariableHandler(updateFleetVariableService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/compose-files/{composeFileID}/variables/{variableID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.DeleteVariableHandler(deleteFleetVariableService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/operations", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, idempotency.Middleware(idempotencyStore, fleethttp.TriggerOperationHandler(triggerOperationService))))
	mux.HandleFunc("GET /api/v1/orgs/{id}/operations", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.ListOperationsHandler(listOperationsService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/operations/{operationID}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.GetOperationHandler(getOperationService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}/operations/{operationID}/stream", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, fleethttp.StreamOperationHandler(getOperationService, redisClient)))

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		// RequestID outermost - so even a 429 rejection from RateLimit
		// carries a real request id (useful for correlating abuse across
		// logs, the exact reason RequestID exists in the first place).
		// otelhttp.NewHandler is innermost, wrapping the mux directly -
		// a real span per request that reaches real routing (a
		// rate-limited 429 is noise, not work worth tracing). This is
		// the actual "distributed" half of tracing: the span this
		// creates is what otelgrpc's client interceptor (cmd/worker)
		// continues across the gRPC call when a request triggers a Run
		// dispatch - one trace, two processes.
		Handler: httpserver.RequestID(httpserver.RateLimit(generalLimiter, otelhttp.NewHandler(mux, "control-plane"))),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Every background Runnable (docs/architecture/18-backend-structure.md
	// §4 - so far just the Outbox Relay, but this is the same supervision
	// shape the doc names for every future one: "starts every registered
	// Runnable in its own goroutine under one errgroup.Group tied to a
	// context that's canceled on SIGTERM") plus the HTTP server itself,
	// under one errgroup so a genuine failure in either one triggers a
	// coordinated shutdown of both, not one silently outliving the other.
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return relay.Run(gctx)
	})

	g.Go(func() error {
		return staleRunReaper.Run(gctx)
	})

	g.Go(func() error {
		return purgeReaper.Run(gctx)
	})

	g.Go(func() error {
		return idempotencyReaper.Run(gctx)
	})

	g.Go(func() error {
		return fleetDeployExecutor.Run(gctx)
	})

	g.Go(func() error {
		return rateLimitGC.Run(gctx)
	})

	g.Go(func() error {
		return workerRegistry.SubscribeCancelForwarding(gctx)
	})

	g.Go(func() error {
		return workerRegistry.Heartbeat(gctx)
	})

	g.Go(func() error {
		logger.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	tlsCreds, err := mtls.ServerCredentials(cfg.TLSCACert, cfg.TLSServerCert, cfg.TLSServerKey)
	if err != nil {
		logger.Error("mtls setup failed", "error", err)
		os.Exit(1)
	}
	// otelgrpc.NewServerHandler continues whatever trace the Worker's own
	// otelgrpc client interceptor started (cmd/worker/main.go) - real
	// cross-process span propagation over the actual gRPC connection,
	// via W3C tracecontext metadata (tracing.Setup's own propagator).
	grpcSrv := grpcserver.NewServer(grpcserver.Creds(tlsCreds), grpcserver.StatsHandler(otelgrpc.NewServerHandler()))
	executionpb.RegisterWorkerServiceServer(grpcSrv, grpcWorkerServer)
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		logger.Error("grpc listen failed", "error", err)
		os.Exit(1)
	}

	g.Go(func() error {
		logger.Info("grpc server starting", "addr", cfg.GRPCAddr)
		return grpcSrv.Serve(grpcListener)
	})

	g.Go(func() error {
		<-gctx.Done()
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		grpcSrv.GracefulStop()
		return server.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

// runMigrations applies every pending migration via golang-migrate
// (docs/architecture/05-database.md §3's "plain versioned SQL migrations,
// not an ORM's auto-migrate" decision), using its own short-lived
// *sql.DB rather than the app's pgxpool - migration needs a single
// connection with its own lock, not a pool.
func runMigrations(cfg config.Config, logger *slog.Logger) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	driver, err := cockroachdb.WithInstance(db, &cockroachdb.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "cockroachdb", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	logger.Info("migrations applied")
	return nil
}

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}
