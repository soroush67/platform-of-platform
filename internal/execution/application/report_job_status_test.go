package application_test

import (
	"context"
	"testing"

	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
)

func applyingRun(locker *fakeWorkspaceLocker, runRepo *fakeRunRepo) *domain.Run {
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	run.Status = domain.RunStatusApplying
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	return run
}

func TestWorkerReportService_AppliedMarksRunAppliedAndUnlocks(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run := applyingRun(locker, runRepo)
	svc := application.NewWorkerReportService(runRepo, locker)

	err := svc.HandleReport(context.Background(), testOrgID, run.ID, testWorkspaceID, "applied", "all good", "")
	if err != nil {
		t.Fatalf("HandleReport: %v", err)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusApplied {
		t.Errorf("expected applied, got %q", got.Status)
	}
	if got.ApplyOutputRef == nil || *got.ApplyOutputRef != "all good" {
		t.Errorf("expected the log line to be stored inline, got %v", got.ApplyOutputRef)
	}
	if locker.isLocked(testWorkspaceID) {
		t.Error("expected the workspace lock to be released")
	}
}

func TestWorkerReportService_FailedMarksRunFailedWithCombinedOutput(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run := applyingRun(locker, runRepo)
	svc := application.NewWorkerReportService(runRepo, locker)

	err := svc.HandleReport(context.Background(), testOrgID, run.ID, testWorkspaceID, "failed", "step 3 running", "exit code 1")
	if err != nil {
		t.Fatalf("HandleReport: %v", err)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusFailed {
		t.Errorf("expected failed, got %q", got.Status)
	}
	want := "step 3 running\n\nerror: exit code 1"
	if got.ApplyOutputRef == nil || *got.ApplyOutputRef != want {
		t.Errorf("expected combined output %q, got %v", want, got.ApplyOutputRef)
	}
}

func TestWorkerReportService_DuplicateReportOnATerminalRunIsANoOp(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run := applyingRun(locker, runRepo)
	svc := application.NewWorkerReportService(runRepo, locker)

	if err := svc.HandleReport(context.Background(), testOrgID, run.ID, testWorkspaceID, "applied", "done", ""); err != nil {
		t.Fatalf("first HandleReport: %v", err)
	}
	// A second, late report for the same already-terminal run - the
	// Worker's own at-least-once report delivery, not a bug.
	if err := svc.HandleReport(context.Background(), testOrgID, run.ID, testWorkspaceID, "failed", "", "late failure"); err != nil {
		t.Fatalf("expected a duplicate report on a terminal run to be a silent no-op, got: %v", err)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusApplied {
		t.Errorf("expected the run to stay in its first-reported terminal status, got %q", got.Status)
	}
}

func TestWorkerReportService_UnknownStatusRejected(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run := applyingRun(locker, runRepo)
	svc := application.NewWorkerReportService(runRepo, locker)

	err := svc.HandleReport(context.Background(), testOrgID, run.ID, testWorkspaceID, "not-a-real-status", "", "")
	if _, ok := err.(*domain.ValidationError); !ok {
		t.Fatalf("expected a *domain.ValidationError, got: %T (%v)", err, err)
	}
}
