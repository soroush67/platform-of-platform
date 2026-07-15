package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"platform-of-platform/internal/execution/adapters/postgres"
	"platform-of-platform/internal/execution/domain"
	"platform-of-platform/internal/platform/dbtest"
)

func TestRunRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	run, err := domain.NewRun(orgID, workspaceID, userID)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.RunStatusQueued || got.WorkspaceID != workspaceID {
		t.Errorf("expected the created run to round-trip, got %+v", got)
	}

	var eventCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'RunQueued' AND payload->>'target_id' = $2`, orgID, run.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected exactly 1 RunQueued event, got %d", eventCount)
	}
}

func TestRunRepository_GetByID_UnknownReturnsNotFound(t *testing.T) {
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, _ := insertOrgProjectWorkspace(t, root)

	_, err := repo.GetByID(context.Background(), orgID, uuid.NewString())
	if !errors.Is(err, domain.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound, got: %v", err)
	}
}

func TestRunRepository_ListByWorkspace_NewestFirst(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	first, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	// created_at both default to now() at INSERT time, close enough
	// together that ordering isn't guaranteed by timing alone - force a
	// real, distinguishable order directly.
	mustExec(t, root, `UPDATE runs SET created_at = created_at - interval '1 minute' WHERE id = $1`, first.ID)

	got, err := repo.ListByWorkspace(ctx, orgID, workspaceID)
	if err != nil {
		t.Fatalf("ListByWorkspace: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Errorf("expected newest-first ordering [second, first], got [%s, %s]", got[0].ID, got[1].ID)
	}
}

func TestRunRepository_Update_WritesTerminalEvent(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	run, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := run.Cancel(); err != nil {
		t.Fatalf("domain Cancel: %v", err)
	}
	if err := repo.Update(ctx, run, userID); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.RunStatusCanceled || got.FinishedAt == nil {
		t.Errorf("expected the update to persist, got %+v", got)
	}

	var eventCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'RunCanceled' AND payload->>'target_id' = $2`, orgID, run.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected exactly 1 RunCanceled event, got %d", eventCount)
	}
}

func TestRunRepository_TryStartApplying(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	run, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	started, err := repo.TryStartApplying(ctx, orgID, run.ID, workspaceID)
	if err != nil {
		t.Fatalf("TryStartApplying (first): %v", err)
	}
	if !started {
		t.Fatal("expected TryStartApplying to succeed on a freshly queued run")
	}

	got, err := repo.GetByID(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.RunStatusApplying || got.StartedAt == nil {
		t.Errorf("expected the run to be applying with StartedAt set, got %+v", got)
	}

	var eventCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'RunApplying' AND payload->>'target_id' = $2`, orgID, run.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected exactly 1 RunApplying event, got %d", eventCount)
	}

	// A second attempt on a Run no longer `queued` must be a real,
	// atomic no-op, not a second transition - this is what makes it safe
	// against the Outbox Relay's own at-least-once redelivery.
	started, err = repo.TryStartApplying(ctx, orgID, run.ID, workspaceID)
	if err != nil {
		t.Fatalf("TryStartApplying (redelivery): %v", err)
	}
	if started {
		t.Error("expected a redelivered TryStartApplying on an already-applying run to fail")
	}
}

// TestRunRepository_TryStartApplying_ConcurrentRaceHasExactlyOneWinner
// is the real regression test for TryStartApplying's own doc comment -
// "a real atomic compare-and-swap." Fires many concurrent calls against
// the same freshly queued Run and asserts exactly one succeeds, the
// property that prevents a redelivered RunQueued event from ever
// double-dispatching the same Run to two Workers.
func TestRunRepository_TryStartApplying_ConcurrentRaceHasExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	run, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const attempts = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started, err := repo.TryStartApplying(ctx, orgID, run.ID, workspaceID)
			if err != nil {
				errs[i] = err
				return
			}
			if started {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("TryStartApplying goroutine %d: %v", i, err)
		}
	}
	if wins != 1 {
		t.Errorf("expected exactly 1 of %d concurrent TryStartApplying calls to win, got %d", attempts, wins)
	}
}

func TestRunRepository_RevertToQueued(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	run, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.TryStartApplying(ctx, orgID, run.ID, workspaceID); err != nil {
		t.Fatalf("TryStartApplying: %v", err)
	}

	if err := repo.RevertToQueued(ctx, orgID, run.ID); err != nil {
		t.Fatalf("RevertToQueued: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.RunStatusQueued {
		t.Errorf("expected the run to be reverted to queued, got %q", got.Status)
	}
	if got.StartedAt != nil {
		t.Error("expected StartedAt to be cleared by RevertToQueued")
	}
}

func TestRunRepository_FindStaleApplyingRuns_RespectsCutoff(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	staleRun, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, staleRun); err != nil {
		t.Fatalf("Create staleRun: %v", err)
	}
	if _, err := repo.TryStartApplying(ctx, orgID, staleRun.ID, workspaceID); err != nil {
		t.Fatalf("TryStartApplying staleRun: %v", err)
	}
	mustExec(t, root, `UPDATE outbox_events SET occurred_at = now() - interval '1 hour' WHERE event_type = 'RunApplying' AND payload->>'target_id' = $1`, staleRun.ID)

	freshRun, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, freshRun); err != nil {
		t.Fatalf("Create freshRun: %v", err)
	}
	if _, err := repo.TryStartApplying(ctx, orgID, freshRun.ID, workspaceID); err != nil {
		t.Fatalf("TryStartApplying freshRun: %v", err)
	}

	cutoff := time.Now().Add(-30 * time.Minute)
	candidates, err := repo.FindStaleApplyingRuns(ctx, cutoff)
	if err != nil {
		t.Fatalf("FindStaleApplyingRuns: %v", err)
	}

	var foundStale, foundFresh bool
	for _, c := range candidates {
		if c.RunID == staleRun.ID {
			foundStale = true
			if c.WorkspaceID != workspaceID || c.OrganizationID != orgID {
				t.Errorf("expected the candidate to carry the real org/workspace ids, got %+v", c)
			}
		}
		if c.RunID == freshRun.ID {
			foundFresh = true
		}
	}
	if !foundStale {
		t.Error("expected the backdated (1h ago) applying run to be found")
	}
	if foundFresh {
		t.Error("expected the just-started applying run NOT to be found (inside the cutoff)")
	}
}

func TestRunRepository_MarkErroredIfStillApplying(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewRunRepository(pool)
	orgID, workspaceID := insertOrgProjectWorkspace(t, root)
	userID := uuid.NewString()

	run, _ := domain.NewRun(orgID, workspaceID, userID)
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.TryStartApplying(ctx, orgID, run.ID, workspaceID); err != nil {
		t.Fatalf("TryStartApplying: %v", err)
	}

	reaped, err := repo.MarkErroredIfStillApplying(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("MarkErroredIfStillApplying: %v", err)
	}
	if !reaped {
		t.Fatal("expected a genuinely-still-applying run to be reaped")
	}

	got, err := repo.GetByID(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != domain.RunStatusErrored || got.FinishedAt == nil {
		t.Errorf("expected the run to be errored with FinishedAt set, got %+v", got)
	}

	// Already terminal (errored, from the call above) - a second reap
	// attempt must be a real no-op, not double-transition.
	reaped, err = repo.MarkErroredIfStillApplying(ctx, orgID, run.ID)
	if err != nil {
		t.Fatalf("MarkErroredIfStillApplying (already terminal): %v", err)
	}
	if reaped {
		t.Error("expected MarkErroredIfStillApplying to no-op on an already-terminal run")
	}
}
