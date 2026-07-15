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

func insertOrg(t *testing.T, root *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'audit-adapter-test-org', $2)`,
		id, "audit-adapter-test-org-"+id[:8])
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, id)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}
