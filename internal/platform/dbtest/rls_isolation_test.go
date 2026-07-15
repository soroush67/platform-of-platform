package dbtest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestRLSIsolation_CrossOrgQueriesReturnZeroRows is
// docs/architecture/20-testing-strategy.md's own named RLS isolation
// test, written for real (a real gap named directly: "هیچ تست خودکاری
// نداریم... تمام verify های این جلسه دستی بود"). Runs against the
// platform_app role and a real CockroachDB cluster - not asserted, not
// mocked. Proves the actual security property this whole codebase's
// multi-tenancy rests on: a session scoped to one Organization cannot
// see another Organization's rows, even when it knows the exact primary
// key to ask for (the deeper, more meaningful claim than "a filtered
// list omits them" - this is "a direct, targeted lookup by id still
// finds nothing").
func TestRLSIsolation_CrossOrgQueriesReturnZeroRows(t *testing.T) {
	ctx := context.Background()
	root := RootPool(t)
	app := AppPool(t)

	orgA := uuid.NewString()
	orgB := uuid.NewString()
	projectB := uuid.NewString()

	// Fixtures via the root connection (bypasses RLS) - setting up two
	// tenants is itself a cross-org operation no single app.current_org_id
	// scope could do.
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'rls-test-org-a', $2)`, orgA, "rls-test-org-a-"+orgA[:8])
	mustExec(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'rls-test-org-b', $2)`, orgB, "rls-test-org-b-"+orgB[:8])
	mustExec(t, root, `INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'rls-test-project-b', 'rls-test-project-b')`, projectB, orgB)
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM projects WHERE id = $1`, projectB)
		mustExec(t, root, `DELETE FROM organizations WHERE id IN ($1, $2)`, orgA, orgB)
	})

	// Scope a real app-role transaction to Org A, the same
	// set_config('app.current_org_id', ..., true) every real repository
	// in this codebase uses.
	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, orgA); err != nil {
		t.Fatalf("set_config: %v", err)
	}

	// Sanity check first: Org A's OWN row must still be visible - if
	// this fails, the test below would "pass" for the wrong reason (RLS
	// blocking everything, not just other tenants).
	var visibleA int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id = $1`, orgA).Scan(&visibleA); err != nil {
		t.Fatalf("query org A: %v", err)
	}
	if visibleA != 1 {
		t.Fatalf("expected org A's own row to be visible in its own scoped session, got count=%d", visibleA)
	}

	// The actual isolation claim: Org B's row, looked up directly by its
	// own primary key, must be invisible from Org A's session.
	var visibleB int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id = $1`, orgB).Scan(&visibleB); err != nil {
		t.Fatalf("query org B: %v", err)
	}
	if visibleB != 0 {
		t.Fatalf("RLS isolation violated: org B's row was visible from org A's scoped session (count=%d)", visibleB)
	}

	// Same claim one level down the hierarchy - a Project belonging to
	// Org B, looked up by its own id, from Org A's scope.
	var visibleProjectB int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id = $1`, projectB).Scan(&visibleProjectB); err != nil {
		t.Fatalf("query project B: %v", err)
	}
	if visibleProjectB != 0 {
		t.Fatalf("RLS isolation violated: org B's project was visible from org A's scoped session (count=%d)", visibleProjectB)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
