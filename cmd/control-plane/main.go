// The Control Plane binary - walking skeleton per docs/architecture/21-deployment.md
// §4's bootstrap sequence: connect to Postgres, migrate on startup, serve HTTP.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/cockroachdb"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"platform-of-platform/internal/platform/config"
	identityhttp "platform-of-platform/internal/identity/adapters/http"
	identitypg "platform-of-platform/internal/identity/adapters/postgres"
	identityapp "platform-of-platform/internal/identity/application"
	tenancyhttp "platform-of-platform/internal/tenancy/adapters/http"
	tenancypg "platform-of-platform/internal/tenancy/adapters/postgres"
	tenancyapp "platform-of-platform/internal/tenancy/application"
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

	pool, err := pgxpool.New(context.Background(), cfg.AppDatabaseURL)
	if err != nil {
		logger.Error("db pool init failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Manual wiring, in one place - docs/architecture/18-backend-structure.md §5's
	// "no DI framework" decision: every dependency is greppable from here.
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	createOrgService := tenancyapp.NewCreateOrganizationService(orgRepo)
	getOrgService := tenancyapp.NewGetOrganizationService(orgRepo)

	userRepo := identitypg.NewUserRepository(pool)
	createUserService := identityapp.NewCreateUserService(userRepo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(pool))
	mux.HandleFunc("POST /api/v1/orgs", tenancyhttp.CreateOrganizationHandler(createOrgService))
	mux.HandleFunc("GET /api/v1/orgs/{id}", tenancyhttp.GetOrganizationHandler(getOrgService))
	mux.HandleFunc("POST /api/v1/users", identityhttp.CreateUserHandler(createUserService))

	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
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
