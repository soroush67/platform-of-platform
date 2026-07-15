package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// insertOrg is a lightweight, raw-SQL organization fixture (not routed
// through tenancy/domain - these tests exercise Identity's own adapters,
// not Tenancy's) for the two Identity tables that do carry a real FK
// into organizations: service_accounts and api_keys.
func insertOrg(t *testing.T, root *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'identity-adapter-test-org', $2)`,
		id, "identity-adapter-test-org-"+id[:8])
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, id) })
	return id
}
