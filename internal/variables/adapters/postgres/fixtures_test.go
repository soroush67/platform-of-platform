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

// insertOrg is a lightweight, raw-SQL organization fixture -
// variables.scope_id is polymorphic (no single FK target across
// organization/project/environment/workspace scopes), so a real
// organizations row is all that's needed to satisfy variables' own FK.
func insertOrg(t *testing.T, root *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'variables-adapter-test-org', $2)`,
		id, "variables-adapter-test-org-"+id[:8])
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM variables WHERE organization_id = $1`, id)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}
