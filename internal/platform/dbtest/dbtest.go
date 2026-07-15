// Package dbtest gives the real, infra-backed test suite
// (docs/architecture/20-testing-strategy.md's own named tests - RLS
// isolation, Outbox atomicity, Stale Run Reaper) one shared way to
// reach a real CockroachDB cluster, root and platform_app both. This
// whole session's own discipline applies to the tests themselves too:
// no mocked database, no sqlmock - these connect to the actual 3-node
// cluster docker-compose.yml stands up, the same way cmd/control-plane
// itself does.
//
// Run these with `go test ./...` from *inside* a container attached to
// the compose network (the cockroach nodes aren't published to the
// host - docker-compose.yml only exposes the Control Plane's own
// ports), e.g.:
//
//	docker run --rm --network platform-of-platform_default \
//	  -v "$PWD":/app -w /app golang:1.25 go test ./...
//
// TEST_DATABASE_URL/TEST_APP_DATABASE_URL override the defaults (which
// match docker-compose.yml's own DATABASE_URL/APP_DATABASE_URL
// verbatim) for anyone running against a differently-named stack.
package dbtest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultRootURL = "postgresql://root@cockroach-0:26257/platform?sslmode=disable"
	defaultAppURL  = "postgresql://platform_app@cockroach-0:26257/platform?sslmode=disable"
)

// RootPool connects as the migration/superuser role - bypasses RLS
// entirely, same as cmd/control-plane's own DatabaseURL connection.
// Used by test fixtures to set up/tear down rows across organizations,
// which a platform_app connection scoped to one org could never do.
func RootPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return connect(t, envOrDefault("TEST_DATABASE_URL", defaultRootURL))
}

// AppPool connects as platform_app - genuinely RLS-constrained, the
// same role and grants cmd/control-plane itself runs every real query
// through. This is the pool RLS isolation tests must use: testing
// isolation against the root connection would prove nothing, since root
// implicitly bypasses RLS (verified for real earlier this session).
func AppPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return connect(t, envOrDefault("TEST_APP_DATABASE_URL", defaultAppURL))
}

func connect(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("dbtest: connecting to %s: %v", url, err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		// Skip, not Fatal: a real CockroachDB cluster genuinely isn't
		// reachable from wherever `go test` is running right now (e.g. on
		// the host, not inside the compose network) - that's a real
		// environment gap this test can't paper over with a mock, so it
		// says so and steps aside rather than reporting a false failure.
		t.Skipf("dbtest: cannot reach %s (%v) - run inside a container on the platform-of-platform_default network, see package doc", url, err)
	}

	t.Cleanup(pool.Close)
	return pool
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
