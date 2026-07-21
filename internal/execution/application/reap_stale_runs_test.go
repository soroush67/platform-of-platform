package application_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	executionpg "platform-of-platform/internal/execution/adapters/postgres"
	executionapp "platform-of-platform/internal/execution/application"
	executiondomain "platform-of-platform/internal/execution/domain"
	"platform-of-platform/internal/platform/dbtest"
	workspacepg "platform-of-platform/internal/workspace/adapters/postgres"
	workspacedomain "platform-of-platform/internal/workspace/domain"
)

// TestStaleRunReaper_ReclaimsAbandonedRun is docs/architecture/
// 20-testing-strategy.md's own named "Stale Run Reaper chaos test" -
// docs/architecture/07-module-execution.md §3's exact scenario: a
// Worker receives a JobAssignment (TryStartApplying succeeds, the Run
// moves to `applying`) and then simply never reports back - crashed,
// network partition, OOM-killed, doesn't matter which. Nothing else in
// this codebase would ever notice on its own; this test proves the
// Reaper actually does, against a real CockroachDB cluster and real
// wall-clock time (a short-but-real staleAfter/interval, not a mocked
// clock) - the Run must end up `errored` and the Workspace's lock must
// actually release.
func TestStaleRunReaper_ReclaimsAbandonedRun(t *testing.T) {
	ctx := context.Background()
	root := dbtest.RootPool(t)

	runRepo := executionpg.NewRunRepository(root)
	workspaceRepo := workspacepg.NewWorkspaceRepository(root)

	orgID := uuid.NewString()
	userID := uuid.NewString()
	mustExecReaper(t, root, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'reaper-test-org', $2)`, orgID, "reaper-test-org-"+orgID[:8])
	t.Cleanup(func() {
		mustExecReaper(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, orgID)
		// The live control-plane container (docker-compose) shares this
		// same database and runs its own real Outbox Relay - the
		// RunApplying/RunQueued events this test's own code writes get
		// consumed by that real RecordEntryService independently of
		// anything this test does, producing real audit_entries rows
		// that reference orgID. Found for real: without this delete, the
		// organizations delete below fails its own FK constraint
		// whenever the compose stack's control-plane happens to win that
		// race (audit_entries_organization_id_fkey).
		mustExecReaper(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, orgID)
		mustExecReaper(t, root, `DELETE FROM runs WHERE organization_id = $1`, orgID)
		mustExecReaper(t, root, `DELETE FROM workspaces WHERE organization_id = $1`, orgID)
		mustExecReaper(t, root, `DELETE FROM projects WHERE organization_id = $1`, orgID)
		mustExecReaper(t, root, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	projectID := uuid.NewString()
	mustExecReaper(t, root, `INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'reaper-test-project', 'reaper-test-project')`, projectID, orgID)

	ws, err := workspacedomain.NewWorkspace(orgID, projectID, nil, "reaper-test-ws", workspacedomain.ExecutionEngineTerraform)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	if err := workspaceRepo.Create(ctx, ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	run, err := executiondomain.NewRun(orgID, ws.ID, userID)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// TryLock is what TriggerRunService does before creating the Run in
	// the real flow - reproduced here so the Reaper's own Unlock call
	// has a real lock to release, not a no-op.
	locked, err := workspaceRepo.TryLock(ctx, orgID, ws.ID, run.ID)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !locked {
		t.Fatal("expected TryLock to succeed on a freshly created, unlocked workspace")
	}

	// The real transition a connected Worker taking the Job would cause
	// (RunDispatchService's own call) - this is the exact moment a Worker
	// that then dies would leave the Run stuck at, forever, without the
	// Reaper.
	started, err := runRepo.TryStartApplying(ctx, orgID, run.ID, ws.ID)
	if err != nil {
		t.Fatalf("TryStartApplying: %v", err)
	}
	if !started {
		t.Fatal("expected TryStartApplying to succeed on a freshly created, queued Run")
	}

	// No MarkApplied/MarkFailed ever follows - the Worker's silence is
	// simulated by simply doing nothing, the same as a real crash would.

	tracker := newFakeRunTracker()
	reaper := executionapp.NewStaleRunReaperService(runRepo, workspaceRepo, tracker, 200*time.Millisecond, 100*time.Millisecond, slog.Default())
	reaperCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	go reaper.Run(reaperCtx)

	// Real wall-clock polling for the real outcome - no channel the
	// Reaper signals on, the same as an operator watching the API would
	// have to do.
	deadline := time.Now().Add(2500 * time.Millisecond)
	var finalStatus string
	for time.Now().Before(deadline) {
		reaped, err := runRepo.GetByID(ctx, orgID, run.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		finalStatus = string(reaped.Status)
		if finalStatus == string(executiondomain.RunStatusErrored) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if finalStatus != string(executiondomain.RunStatusErrored) {
		t.Fatalf("expected the abandoned Run to be reaped to 'errored' within the deadline, last observed status: %q", finalStatus)
	}

	// The other half of reapOnce's real work: the Workspace lock must
	// actually be released, not just the Run's own status flipped - a
	// second TryLock on the same workspace must now succeed. Deliberately
	// checked before the tracker.wasForgotten assertion below: reapOnce
	// calls Unlock, then Forget, strictly in that program order within
	// the same synchronous call, but the polling loop above only proves
	// the *status* write (which happens even earlier, via
	// MarkErroredIfStillApplying) landed - there's a real window where
	// the goroutine running reapOnce hasn't reached Unlock/Forget yet
	// even though the status is already visible. This second TryLock's
	// own real DB round trip is what gives that goroutine time to
	// actually get there, the same way the original version of this test
	// already relied on for Unlock alone before Forget existed.
	otherRun, err := executiondomain.NewRun(orgID, ws.ID, userID)
	if err != nil {
		t.Fatalf("NewRun (second): %v", err)
	}
	if err := runRepo.Create(ctx, otherRun); err != nil {
		t.Fatalf("create second run: %v", err)
	}
	relocked, err := workspaceRepo.TryLock(ctx, orgID, ws.ID, otherRun.ID)
	if err != nil {
		t.Fatalf("TryLock (second): %v", err)
	}
	if !relocked {
		t.Fatal("expected the Workspace lock to be released by the Reaper - a second TryLock should have succeeded")
	}

	if !tracker.wasForgotten(run.ID) {
		t.Error("expected the reaper to forget the abandoned run's Cancel-routing entry too, not just unlock its workspace")
	}
}

func mustExecReaper(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
