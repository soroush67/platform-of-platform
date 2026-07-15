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

// insertOrgAndProject is a lightweight, raw-SQL fixture - these tests
// exercise Workspace's own adapters, not Tenancy's, and both
// environments/workspaces have a real FK into projects (which itself
// FKs into organizations), so both are needed just to satisfy schema
// constraints.
func insertOrgAndProject(t *testing.T, root *pgxpool.Pool) (orgID, projectID string) {
	t.Helper()
	orgID = uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'workspace-adapter-test-org', $2)`,
		orgID, "workspace-adapter-test-org-"+orgID[:8])

	projectID = uuid.NewString()
	mustExec(t, root, `INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'workspace-adapter-test-project', $3)`,
		projectID, orgID, "workspace-adapter-test-project-"+projectID[:8])

	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM projects WHERE id = $1`, projectID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return orgID, projectID
}
