// Package dbtest gives the real, infra-backed test suite
// (docs/architecture/20-testing-strategy.md's own named tests - RLS
// isolation, Outbox atomicity, Stale Run Reaper) one shared way to
// reach real infra: a real CockroachDB cluster (root and platform_app
// both) and the real Redis instance the Worker Registry's multi-
// instance HA routing depends on. This whole session's own discipline
// applies to the tests themselves too: no mocked database, no sqlmock,
// no miniredis - these connect to the actual services docker-compose.yml
// stands up, the same way cmd/control-plane itself does.
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
	"github.com/redis/go-redis/v9"
)

const (
	defaultRootURL  = "postgresql://root@cockroach-0:26257/platform?sslmode=disable"
	defaultAppURL   = "postgresql://platform_app@cockroach-0:26257/platform?sslmode=disable"
	defaultRedisURL = "redis:6379"
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

// RedisClient connects to the same real redis:7-alpine service
// docker-compose.yml stands up for the Worker Registry's multi-instance
// HA routing (internal/execution/adapters/grpc) - same "real infra, no
// mock" posture as RootPool/AppPool above, just for the one non-SQL
// dependency this codebase has. TEST_REDIS_ADDR overrides the default
// for anyone running against a differently-named stack.
func RedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := envOrDefault("TEST_REDIS_ADDR", defaultRedisURL)
	client := redis.NewClient(&redis.Options{Addr: addr})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		t.Skipf("dbtest: cannot reach redis at %s (%v) - run inside a container on the platform-of-platform_default network, see package doc", addr, err)
	}

	t.Cleanup(func() { client.Close() })
	return client
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

// DeleteOrganization retry-deletes audit_entries then organizations for
// orgID - the real, closed-loop fix for a genuine TOCTOU race this test
// suite hit repeatedly: the live compose stack's own control-plane
// container shares this same database and runs its own real Outbox
// Relay, which can record a brand new audit_entries row for a test
// org's outbox event *between* this helper's own audit_entries delete
// and its organizations delete - a plain "delete audit_entries first,
// then organizations" ordering (tried first, insufficient) can't close
// this, since it's racing an external, unsynchronized process, not a
// same-transaction ordering problem. Retrying the whole
// delete-audit_entries-then-organizations pair a few times with a short
// backoff is what actually closes it: the live Relay's own poll
// interval is a real, bounded 2s (docs/architecture/18-backend-
// structure.md's own Relay interval), so a few retries across ~1.5s
// reliably outlasts one unlucky poll cycle. Callers should still delete
// every other org-scoped row (outbox_events, runs, projects, etc.)
// themselves first - this only owns the audit_entries+organizations
// tail end of that sequence, the specific pair this race is about.
func DeleteOrganization(t *testing.T, root *pgxpool.Pool, orgID string) {
	t.Helper()
	ctx := context.Background()

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		if _, err := root.Exec(ctx, `DELETE FROM audit_entries WHERE organization_id = $1`, orgID); err != nil {
			lastErr = err
			continue
		}
		if _, err := root.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID); err != nil {
			lastErr = err
			continue
		}
		return
	}
	t.Fatalf("dbtest: DeleteOrganization(%s) failed after retries: %v", orgID, lastErr)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
