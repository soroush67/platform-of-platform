// Package config loads the Control Plane's runtime configuration from
// environment variables, per Stage 1's Twelve-Factor principle (docs/
// architecture/01-architecture-style-and-challenges.md) and Stage 21's
// bootstrap sequence (docs/architecture/21-deployment.md §4).
package config

import (
	"fmt"
	"os"
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
	AppDatabaseURL    string
	HTTPAddr          string
	InitialAdminEmail string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		AppDatabaseURL:    os.Getenv("APP_DATABASE_URL"),
		HTTPAddr:          getenvDefault("HTTP_ADDR", ":8443"),
		InitialAdminEmail: os.Getenv("INITIAL_PLATFORM_ADMIN_EMAIL"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.AppDatabaseURL == "" {
		return Config{}, fmt.Errorf("config: APP_DATABASE_URL is required")
	}

	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
