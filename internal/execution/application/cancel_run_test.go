package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
)

func newCancelRunService() (*application.CancelRunService, *fakeRunRepo, *fakeWorkspaceLocker, *fakeScopedPermissionChecker, *fakeWorkerCanceler) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	permChecker := newFakeScopedPermissionChecker()
	canceler := newFakeWorkerCanceler()
	svc := application.NewCancelRunService(runRepo, locker, permChecker, canceler)
	return svc, runRepo, locker, permChecker, canceler
}

func TestCancelRunService_RequiresWorkspaceApply(t *testing.T) {
	svc, _, _, _, _ := newCancelRunService()

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "run-1", "user-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without workspace:apply, got: %v", err)
	}
}

func TestCancelRunService_RejectsRunFromAnotherWorkspace(t *testing.T) {
	svc, runRepo, locker, permChecker, _ := newCancelRunService()
	permChecker.grant(testOrgID, "user-1", "workspace:apply")
	run, _ := domain.NewRun(testOrgID, "a-different-workspace", "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, "a-different-workspace", run.ID)

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, run.ID, "user-1")
	if !errors.Is(err, domain.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for a run under a different workspace, got: %v", err)
	}
}

func TestCancelRunService_AlreadyTerminalRunRejected(t *testing.T) {
	svc, runRepo, locker, permChecker, _ := newCancelRunService()
	permChecker.grant(testOrgID, "user-1", "workspace:apply")
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	_ = run.Cancel() // already terminal
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, run.ID, "user-1")
	if !errors.Is(err, domain.ErrRunAlreadyTerminal) {
		t.Fatalf("expected ErrRunAlreadyTerminal, got: %v", err)
	}
}

func TestCancelRunService_SucceedsUnlocksAndNotifiesTheWorker(t *testing.T) {
	svc, runRepo, locker, permChecker, canceler := newCancelRunService()
	permChecker.grant(testOrgID, "user-1", "workspace:apply")
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)

	got, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, run.ID, "user-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Status != domain.RunStatusCanceled {
		t.Errorf("expected the run to be canceled, got %q", got.Status)
	}
	if locker.isLocked(testWorkspaceID) {
		t.Error("expected the workspace lock to be released")
	}
	if len(canceler.canceled) != 1 || canceler.canceled[0] != run.ID {
		t.Errorf("expected the worker canceler to be notified of run %s, got %v", run.ID, canceler.canceled)
	}
}
