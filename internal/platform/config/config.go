// Package config loads the Control Plane's runtime configuration from
// environment variables, per Stage 1's Twelve-Factor principle (docs/
// architecture/01-architecture-style-and-challenges.md) and Stage 21's
// bootstrap sequence (docs/architecture/21-deployment.md §4).
package config

import (
	"encoding/hex"
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
	// IdempotencyReaperInterval configures the Idempotency-Key Reaper
	// (internal/platform/idempotency/reaper.go) - how often it sweeps
	// idempotency_keys for rows past idempotency.Window (a fixed 24h,
	// not configurable - docs/architecture/04-api-design.md §5's own
	// contract value, not a tunable). Same "small value for real
	// verification, set via env" posture as RunReaperInterval/
	// OrgPurgeReaperInterval above.
	IdempotencyReaperInterval time.Duration
	// RedisAddr is the multi-instance HA coordination store
	// (internal/execution/adapters/grpc's Registry - runID/Worker
	// routing shared across Control Plane replicas, docs/architecture/
	// 05-database.md §5's own "cache/coordination, never
	// system-of-record" role for Redis). Required, no in-memory
	// fallback - a Registry silently falling back to process-local-only
	// routing would be exactly the "looks like it works, actually
	// doesn't span replicas" failure mode this config existing to
	// enforce is meant to prevent, same "fail closed" posture already
	// applied to the TLS cert trio below.
	RedisAddr string
	// SecretsMasterKey is the envelope-encryption master key
	// (internal/platform/envelope.KeySize, 32 raw bytes, hex-encoded in
	// the env var) every SecretMount's own AppRole secret_id is sealed
	// under before it ever reaches Postgres (docs/architecture/11-
	// module-secrets-state.md §1). Required, no generated-at-boot
	// fallback - a random-per-boot key would make every previously
	// sealed secret_id permanently undecryptable the moment the process
	// restarts, silently bricking every existing SecretMount; failing
	// closed here is the same posture as the TLS cert trio and RedisAddr
	// above.
	SecretsMasterKey []byte
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
	idempotencyReaperInterval, err := time.ParseDuration(getenvDefault("IDEMPOTENCY_REAPER_INTERVAL", "1h"))
	if err != nil {
		return Config{}, fmt.Errorf("config: IDEMPOTENCY_REAPER_INTERVAL: %w", err)
	}

	cfg := Config{
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		AppDatabaseURL:            os.Getenv("APP_DATABASE_URL"),
		HTTPAddr:                  getenvDefault("HTTP_ADDR", ":8443"),
		GRPCAddr:                  getenvDefault("GRPC_ADDR", ":9000"),
		InitialAdminEmail:         os.Getenv("INITIAL_PLATFORM_ADMIN_EMAIL"),
		JWTSigningKey:             []byte(jwtKey),
		RunStaleAfter:             staleAfter,
		RunReaperInterval:         reaperInterval,
		TLSCACert:                 os.Getenv("TLS_CA_CERT"),
		TLSServerCert:             os.Getenv("TLS_SERVER_CERT"),
		TLSServerKey:              os.Getenv("TLS_SERVER_KEY"),
		OrgPurgeAfter:             orgPurgeAfter,
		OrgPurgeReaperInterval:    orgPurgeReaperInterval,
		IdempotencyReaperInterval: idempotencyReaperInterval,
		RedisAddr:                 os.Getenv("REDIS_ADDR"),
	}

	secretsMasterKeyHex := os.Getenv("SECRETS_MASTER_KEY")
	if secretsMasterKeyHex == "" {
		return Config{}, fmt.Errorf("config: SECRETS_MASTER_KEY is required - SecretMount credentials have no plaintext fallback")
	}
	secretsMasterKey, err := hex.DecodeString(secretsMasterKeyHex)
	if err != nil {
		return Config{}, fmt.Errorf("config: SECRETS_MASTER_KEY must be hex-encoded: %w", err)
	}
	if len(secretsMasterKey) != 32 {
		return Config{}, fmt.Errorf("config: SECRETS_MASTER_KEY must decode to exactly 32 bytes, got %d", len(secretsMasterKey))
	}
	cfg.SecretsMasterKey = secretsMasterKey

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
	if cfg.RedisAddr == "" {
		return Config{}, fmt.Errorf("config: REDIS_ADDR is required - the Worker Registry has no in-memory-only fallback for multi-instance routing")
	}

	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
