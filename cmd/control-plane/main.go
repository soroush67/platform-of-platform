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
	workerRegistry := executiongrpc.NewRegistry()

	runRepo := executionpg.NewRunRepository(pool)
	triggerRunService := executionapp.NewTriggerRunService(runRepo, workspaceRepo, workspaceRepo, roleBindingRepo, orgRepo)
	cancelRunService := executionapp.NewCancelRunService(runRepo, workspaceRepo, roleBindingRepo, workerRegistry)
	listRunsService := executionapp.NewListRunsService(runRepo, membershipRepo, workspaceRepo)
	getRunService := executionapp.NewGetRunService(runRepo, membershipRepo, workspaceRepo)
	workerReportService := executionapp.NewWorkerReportService(runRepo, workspaceRepo, workerRegistry)
	staleRunReaper := executionapp.NewStaleRunReaperService(runRepo, workspaceRepo, workerRegistry, cfg.RunStaleAfter, cfg.RunReaperInterval, logger)
	purgeReaper := tenancyapp.NewPurgeReaperService(orgRepo, cfg.OrgPurgeAfter, cfg.OrgPurgeReaperInterval, logger)

	variableRepo := variablespg.NewVariableRepository(pool)
	createVariableService := variablesapp.NewCreateVariableService(variableRepo, membershipRepo, projectRepo, environmentRepo, workspaceRepo, roleBindingRepo, orgRepo)
	listVariablesService := variablesapp.NewListVariablesService(variableRepo, membershipRepo)
	resolveVariableService := variablesapp.NewResolveVariableService(variableRepo, membershipRepo, workspaceRepo)
	updateVariableService := variablesapp.NewUpdateVariableService(variableRepo, membershipRepo, roleBindingRepo)
	deleteVariableService := variablesapp.NewDeleteVariableService(variableRepo, membershipRepo, roleBindingRepo)

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
	mux.HandleFunc("POST /api/v1/users", identityhttp.CreateUserHandler(createUserService))
	mux.HandleFunc("POST /api/v1/auth/login", identityhttp.LoginHandler(authenticateService, refreshTokenService, loginLimiter, cfg.JWTSigningKey))
	mux.HandleFunc("POST /api/v1/auth/refresh", identityhttp.RefreshTokenHandler(refreshTokenService, cfg.JWTSigningKey))
	mux.HandleFunc("POST /api/v1/auth/password-reset/request", identityhttp.RequestPasswordResetHandler(passwordResetService))
	mux.HandleFunc("POST /api/v1/auth/password-reset/confirm", identityhttp.ConfirmPasswordResetHandler(passwordResetService))
	mux.HandleFunc("POST /api/v1/orgs", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.CreateOrganizationHandler(createOrgService)))
	mux.HandleFunc("GET /api/v1/orgs/{id}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.GetOrganizationHandler(getOrgService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/members", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, tenancyhttp.AddMemberHandler(addMemberService)))
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
	mux.HandleFunc("GET /api/v1/orgs/{id}/audit-log", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, audithttp.ListAuditLogHandler(listAuditEntriesService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/service-accounts", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.CreateServiceAccountHandler(createServiceAccountService)))
	mux.HandleFunc("POST /api/v1/orgs/{id}/service-accounts/{sa}/api-keys", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.CreateAPIKeyHandler(createAPIKeyService)))
	mux.HandleFunc("DELETE /api/v1/orgs/{id}/service-accounts/{sa}/api-keys/{key}", httpserver.RequireAuth(cfg.JWTSigningKey, apiKeyResolver, identityhttp.RevokeAPIKeyHandler(revokeAPIKeyService)))

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
		return rateLimitGC.Run(gctx)
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
