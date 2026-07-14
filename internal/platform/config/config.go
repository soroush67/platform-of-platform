// Package config loads the Control Plane's runtime configuration from
// environment variables, per Stage 1's Twelve-Factor principle (docs/
// architecture/01-architecture-style-and-challenges.md) and Stage 21's
// bootstrap sequence (docs/architecture/21-deployment.md §4).
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	// DatabaseURL connects as the migration/superuser role (root) - used
	// only to apply schema migrations, which must bypass RLS to create
	// the policies in the first place (verified against a real
	// CockroachDB node: root implicitly bypasses RLS, a non-superuser
	// role does not).
	DatabaseURL string
	// AppDatabaseURL connects as the non-superuser `platform_app` role
	// (migrations/0001_init.up.sql) - every runtime query the Control
	// Plane itself issues goes through this connection, so RLS actually
	// constrains it per docs/architecture/05-database.md §1.
	AppDatabaseURL string
	HTTPAddr       string
	// GRPCAddr is where the Control Plane listens for Worker connections
	// (docs/architecture/21-deployment.md §1's own
	// CONTROL_PLANE_GRPC_ADDR: control-plane:9000 for the Worker side of
	// this same setting).
	GRPCAddr          string
	InitialAdminEmail string
	// JWTSigningKey signs/verifies access tokens (internal/platform/auth) -
	// same bootstrap-secret posture as MASTER_KEY in
	// docs/architecture/21-deployment.md §1: a real deployment sources
	// this from a real secret store, not a committed default.
	JWTSigningKey []byte
	// RunStaleAfter/RunReaperInterval configure the Stale Run Reaper
	// (internal/execution/application/reap_stale_runs.go) - how long a
	// Run may sit in `applying` before it's considered abandoned, and
	// how often the sweep runs. Defaults are production-shaped (5
	// minutes, 30 seconds); real verification of this feature needs
	// much smaller values, set via env, not hardcoded into the binary.
	RunStaleAfter     time.Duration
	RunReaperInterval time.Duration
	// TLSCACert/TLSServerCert/TLSServerKey: real mTLS for the Worker<->
	// Control Plane gRPC channel (docs/architecture/17-workers.md's own
	// "worker identity token" - the previous insecure.NewCredentials()
	// setup was explicitly flagged dev-only). All three required
	// together: the server presents TLSServerCert/Key and verifies every
	// connecting Worker's client cert against TLSCACert
	// (tls.RequireAndVerifyClientCert) - a Worker without a cert signed
	// by this CA can't connect at all, not just "connects but is
	// untrusted."
	TLSCACert     string
	TLSServerCert string
	TLSServerKey  string
	// OrgPurgeAfter/OrgPurgeReaperInterval configure the Purge Reaper
	// (internal/tenancy/application/purge_reaper.go) - how long an
	// Organization may sit `archived` before it's hard-deleted, and how
	// often the sweep runs. Default matches docs/architecture/13-module-
	// identity-rbac-tenancy.md §1's own "30 days out"; real verification
	// needs a much smaller value, set via env, same posture as
	// RunStaleAfter/RunReaperInterval above.
	OrgPurgeAfter          time.Duration
	OrgPurgeReaperInterval time.Duration
}

func Load() (Config, error) {
	jwtKey := os.Getenv("JWT_SIGNING_KEY")

	staleAfter, err := time.ParseDuration(getenvDefault("RUN_STALE_AFTER", "5m"))
	if err != nil {
		return Config{}, fmt.Errorf("config: RUN_STALE_AFTER: %w", err)
	}
	reaperInterval, err := time.ParseDuration(getenvDefault("RUN_REAPER_INTERVAL", "30s"))
	if err != nil {
		return Config{}, fmt.Errorf("config: RUN_REAPER_INTERVAL: %w", err)
	}
	orgPurgeAfter, err := time.ParseDuration(getenvDefault("ORG_PURGE_AFTER", "720h"))
	if err != nil {
		return Config{}, fmt.Errorf("config: ORG_PURGE_AFTER: %w", err)
	}
	orgPurgeReaperInterval, err := time.ParseDuration(getenvDefault("ORG_PURGE_REAPER_INTERVAL", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("config: ORG_PURGE_REAPER_INTERVAL: %w", err)
	}

	cfg := Config{
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		AppDatabaseURL:         os.Getenv("APP_DATABASE_URL"),
		HTTPAddr:               getenvDefault("HTTP_ADDR", ":8443"),
		GRPCAddr:               getenvDefault("GRPC_ADDR", ":9000"),
		InitialAdminEmail:      os.Getenv("INITIAL_PLATFORM_ADMIN_EMAIL"),
		JWTSigningKey:          []byte(jwtKey),
		RunStaleAfter:          staleAfter,
		RunReaperInterval:      reaperInterval,
		TLSCACert:              os.Getenv("TLS_CA_CERT"),
		TLSServerCert:          os.Getenv("TLS_SERVER_CERT"),
		TLSServerKey:           os.Getenv("TLS_SERVER_KEY"),
		OrgPurgeAfter:          orgPurgeAfter,
		OrgPurgeReaperInterval: orgPurgeReaperInterval,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.AppDatabaseURL == "" {
		return Config{}, fmt.Errorf("config: APP_DATABASE_URL is required")
	}
	if jwtKey == "" {
		return Config{}, fmt.Errorf("config: JWT_SIGNING_KEY is required")
	}
	if cfg.TLSCACert == "" || cfg.TLSServerCert == "" || cfg.TLSServerKey == "" {
		return Config{}, fmt.Errorf("config: TLS_CA_CERT, TLS_SERVER_CERT, and TLS_SERVER_KEY are all required - the Worker gRPC channel has no insecure fallback")
	}

	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
