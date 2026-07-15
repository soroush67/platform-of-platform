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

// insertUser creates a real users row via the root connection - Users
// are platform-global with no RLS (docs/architecture/03-domain-model.md
// §3), and organization_memberships/team_memberships both have a real
// foreign key into this table, so a membership fixture needs a genuine
// row here, not just a random UUID.
func insertUser(t *testing.T, root *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	mustExec(t, root, `INSERT INTO users (id, username, email, auth_source) VALUES ($1, $2, $3, 'local')`,
		id, "adaptertest-"+id[:8], "adaptertest-"+id[:8]+"@example.com")
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, id) })
	return id
}
