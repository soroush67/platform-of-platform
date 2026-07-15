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

// insertOrgProjectWorkspace is a lightweight, raw-SQL fixture chain -
// runs.workspace_id has a real FK into workspaces, which itself FKs
// into projects and organizations, so all three are needed just to
// satisfy schema constraints for a Run row.
func insertOrgProjectWorkspace(t *testing.T, root *pgxpool.Pool) (orgID, workspaceID string) {
	t.Helper()
	orgID = uuid.NewString()
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'execution-adapter-test-org', $2)`,
		orgID, "execution-adapter-test-org-"+orgID[:8])

	projectID := uuid.NewString()
	mustExec(t, root, `INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'execution-adapter-test-project', $3)`,
		projectID, orgID, "execution-adapter-test-project-"+projectID[:8])

	workspaceID = uuid.NewString()
	mustExec(t, root, `INSERT INTO workspaces (id, organization_id, project_id, name, execution_engine, locked)
		VALUES ($1, $2, $3, 'execution-adapter-test-ws', 'compose', false)`,
		workspaceID, orgID, projectID)

	t.Cleanup(func() {
		// Every test in this package creates its own Run rows (and the
		// outbox events Create/TryStartApplying/Update write alongside
		// them) directly against this fixture's org/workspace, without
		// registering their own cleanup - deleting both here first,
		// scoped by organization_id, is what lets the workspace delete
		// below succeed instead of hitting runs_workspace_id_fkey.
		//
		// audit_entries is deleted too, before organizations - the live
		// compose stack's real control-plane container shares this same
		// database and runs its own real Outbox Relay, which can record a
		// genuine audit_entries row for these test orgs' RunQueued/
		// RunApplying events independently of anything this test does.
		// Found for real: without this, the organizations delete below
		// intermittently fails audit_entries_organization_id_fkey
		// whenever that real Relay wins the race (same class of bug fixed
		// once already in execution/application/reap_stale_runs_test.go).
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, orgID)
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, orgID)
		mustExec(t, root, `DELETE FROM runs WHERE organization_id = $1`, orgID)
		mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, workspaceID)
		mustExec(t, root, `DELETE FROM projects WHERE id = $1`, projectID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return orgID, workspaceID
}
