package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	"platform-of-platform/internal/workspace/adapters/postgres"
	"platform-of-platform/internal/workspace/domain"
)

func TestWorkspaceRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, err := domain.NewWorkspace(orgID, projectID, nil, "my-workspace", domain.ExecutionEngineTerraform)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	got, err := repo.GetByID(ctx, orgID, ws.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "my-workspace" || got.ExecutionEngine != domain.ExecutionEngineTerraform {
		t.Errorf("expected fields to round-trip, got %+v", got)
	}
	if got.Locked {
		t.Error("expected a freshly created workspace to be unlocked")
	}
}

func TestWorkspaceRepository_GetByID_WrongOrganizationReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, _ := domain.NewWorkspace(orgID, projectID, nil, "my-workspace", domain.ExecutionEngineTerraform)
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	_, err := repo.GetByID(ctx, uuid.NewString(), ws.ID)
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a workspace under a different org, got: %v", err)
	}
}

func TestWorkspaceRepository_GetExecutionEngine(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, _ := domain.NewWorkspace(orgID, projectID, nil, "engine-ws", domain.ExecutionEngineAnsible)
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	engine, err := repo.GetExecutionEngine(ctx, orgID, ws.ID)
	if err != nil {
		t.Fatalf("GetExecutionEngine: %v", err)
	}
	if engine != string(domain.ExecutionEngineAnsible) {
		t.Errorf("expected engine %q, got %q", domain.ExecutionEngineAnsible, engine)
	}
}

func TestWorkspaceRepository_WorkspaceExistsAndExists(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, _ := domain.NewWorkspace(orgID, projectID, nil, "exists-ws", domain.ExecutionEngineTerraform)
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	exists, err := repo.WorkspaceExists(ctx, orgID, projectID, ws.ID)
	if err != nil {
		t.Fatalf("WorkspaceExists: %v", err)
	}
	if !exists {
		t.Error("expected the workspace to exist under its real project")
	}

	exists, err = repo.WorkspaceExists(ctx, orgID, uuid.NewString(), ws.ID)
	if err != nil {
		t.Fatalf("WorkspaceExists (wrong project): %v", err)
	}
	if exists {
		t.Error("expected the workspace to not exist under an unrelated project id")
	}

	exists, err = repo.Exists(ctx, orgID, ws.ID)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("expected Exists (the project-agnostic check) to find the real workspace")
	}
}

func TestWorkspaceRepository_GetScope(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, _ := domain.NewWorkspace(orgID, projectID, nil, "scope-ws", domain.ExecutionEngineTerraform)
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	gotProjectID, gotEnvironmentID, err := repo.GetScope(ctx, orgID, ws.ID)
	if err != nil {
		t.Fatalf("GetScope: %v", err)
	}
	if gotProjectID != projectID {
		t.Errorf("expected projectID %q, got %q", projectID, gotProjectID)
	}
	if gotEnvironmentID != nil {
		t.Errorf("expected a nil environmentID for a workspace created without one, got %v", gotEnvironmentID)
	}
}

func TestWorkspaceRepository_TryLockAndUnlock(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, _ := domain.NewWorkspace(orgID, projectID, nil, "lock-ws", domain.ExecutionEngineTerraform)
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	runA := uuid.NewString()
	locked, err := repo.TryLock(ctx, orgID, ws.ID, runA)
	if err != nil {
		t.Fatalf("TryLock (runA): %v", err)
	}
	if !locked {
		t.Fatal("expected TryLock to succeed on a freshly created, unlocked workspace")
	}

	runB := uuid.NewString()
	locked, err = repo.TryLock(ctx, orgID, ws.ID, runB)
	if err != nil {
		t.Fatalf("TryLock (runB): %v", err)
	}
	if locked {
		t.Error("expected TryLock to fail while another run holds the lock")
	}

	// Unlock's own guard: a stray Unlock from a run that never actually
	// held the lock must not release someone else's lock.
	if err := repo.Unlock(ctx, orgID, ws.ID, runB); err != nil {
		t.Fatalf("Unlock (wrong run): %v", err)
	}
	got, err := repo.GetByID(ctx, orgID, ws.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.Locked {
		t.Error("expected the workspace to remain locked - runB never held the lock, its Unlock must be a no-op")
	}

	if err := repo.Unlock(ctx, orgID, ws.ID, runA); err != nil {
		t.Fatalf("Unlock (correct run): %v", err)
	}
	got, err = repo.GetByID(ctx, orgID, ws.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Locked {
		t.Error("expected the workspace to be unlocked after the real lock holder calls Unlock")
	}
}

// TestWorkspaceRepository_TryLock_ConcurrentRaceHasExactlyOneWinner is
// the real regression test for TryLock's own doc comment - "a real
// atomic compare-and-swap... via `SELECT ... FOR UPDATE`" is a claim
// about concurrent behavior that a sequential test can't actually prove.
// Fires 10 real, concurrent TryLock calls against the same freshly
// created workspace from 10 real goroutines (each over its own
// connection, borrowed from the same pool) and asserts exactly one
// wins - the property this whole locking mechanism exists for.
func TestWorkspaceRepository_TryLock_ConcurrentRaceHasExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws, _ := domain.NewWorkspace(orgID, projectID, nil, "race-ws", domain.ExecutionEngineTerraform)
	if err := repo.Create(ctx, ws); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM workspaces WHERE id = $1`, ws.ID) })

	const attempts = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			locked, err := repo.TryLock(ctx, orgID, ws.ID, uuid.NewString())
			if err != nil {
				errs[i] = err
				return
			}
			if locked {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("TryLock goroutine %d: %v", i, err)
		}
	}
	if wins != 1 {
		t.Errorf("expected exactly 1 of %d concurrent TryLock calls to win, got %d", attempts, wins)
	}
}

func TestWorkspaceRepository_ListByProject(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewWorkspaceRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	ws1, _ := domain.NewWorkspace(orgID, projectID, nil, "ws-one", domain.ExecutionEngineTerraform)
	ws2, _ := domain.NewWorkspace(orgID, projectID, nil, "ws-two", domain.ExecutionEngineTerraform)
	if err := repo.Create(ctx, ws1); err != nil {
		t.Fatalf("Create ws1: %v", err)
	}
	if err := repo.Create(ctx, ws2); err != nil {
		t.Fatalf("Create ws2: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM workspaces WHERE id IN ($1, $2)`, ws1.ID, ws2.ID)
	})

	got, err := repo.ListByProject(ctx, orgID, projectID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(got))
	}

	otherOrgWorkspaces, err := repo.ListByProject(ctx, uuid.NewString(), projectID)
	if err != nil {
		t.Fatalf("ListByProject (other org): %v", err)
	}
	if len(otherOrgWorkspaces) != 0 {
		t.Errorf("expected zero workspaces for an unrelated organization, got %d", len(otherOrgWorkspaces))
	}
}
