package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
)

func TestGetRunService_NonMemberGetsWorkspaceNotFoundNotForbidden(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker)

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "run-1", "stranger")
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a non-member, got: %v", err)
	}
}

func TestGetRunService_RejectsRunFromAnotherWorkspace(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	run, _ := domain.NewRun(testOrgID, "a-different-workspace", "member-1")
	runRepo.put(run)
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker)

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, run.ID, "member-1")
	if !errors.Is(err, domain.ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for a run under a different workspace, got: %v", err)
	}
}

func TestGetRunService_Succeeds(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "member-1")
	runRepo.put(run)
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker)

	got, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, run.ID, "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("expected run %s, got %s", run.ID, got.ID)
	}
}

func TestListRunsService_ScopedToWorkspace(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	inWorkspace, _ := domain.NewRun(testOrgID, testWorkspaceID, "member-1")
	otherWorkspace, _ := domain.NewRun(testOrgID, "other-workspace", "member-1")
	runRepo.put(inWorkspace)
	runRepo.put(otherWorkspace)
	svc := application.NewListRunsService(runRepo, membership, workspaceChecker)

	got, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != inWorkspace.ID {
		t.Errorf("expected exactly the one run in this workspace, got %+v", got)
	}
}

func TestListRunsService_NonMemberGetsWorkspaceNotFound(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	svc := application.NewListRunsService(runRepo, membership, workspaceChecker)

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "stranger")
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a non-member, got: %v", err)
	}
}
