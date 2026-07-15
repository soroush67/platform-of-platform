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

// insertOrg is a lightweight, raw-SQL organization fixture - these
// tests exercise RBAC's own adapters, not Tenancy's, so a real
// organizations row (satisfying role_bindings/roles' own FK) is all
// that's needed here, not a real tenancy/domain.Organization.
func insertOrg(t *testing.T, root *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'rbac-adapter-test-org', $2)`,
		id, "rbac-adapter-test-org-"+id[:8])
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, id) })
	return id
}

// insertUser is a lightweight, raw-SQL users fixture -
// organization_memberships/team_memberships aren't involved in these
// tests at all (role_bindings.subject_id has no FK - see migrations/
// 0001_init.up.sql's own CHECK-only constraint), but HasPermissionAtScope's
// team-mediated path does need a real users row for its own
// team_memberships.user_id FK.
func insertUser(t *testing.T, root *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, root, `INSERT INTO users (id, username, email, auth_source) VALUES ($1, $2, $3, 'local')`,
		id, "rbac-adaptertest-"+id[:8], "rbac-adaptertest-"+id[:8]+"@example.com")
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, id) })
	return id
}
