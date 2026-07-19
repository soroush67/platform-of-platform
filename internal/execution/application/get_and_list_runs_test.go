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
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), newFakeVisibilityChecker())

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "run-1", "stranger")
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a non-member, got: %v", err)
	}
}

func TestGetRunService_RequiresVisibilityGrant(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "member-1")
	runRepo.put(run)

	// A real member with no visibility grant at all no longer sees this
	// Run by default (the whole point of this session's per-project
	// visibility change).
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), newFakeVisibilityChecker())
	if _, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, run.ID, "member-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without a visibility grant, got: %v", err)
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
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker, permChecker, newFakeVisibilityChecker())

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
	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(testOrgID, "member-1", "project:read", "project", testProjectID)
	svc := application.NewGetRunService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), visibilityChecker)

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
	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(testOrgID, "member-1", "project:read", "project", testProjectID)
	svc := application.NewListRunsService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), visibilityChecker)

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
	svc := application.NewListRunsService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), newFakeVisibilityChecker())

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "stranger")
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a non-member, got: %v", err)
	}
}

func TestListRunsService_ForbiddenWithoutVisibilityGrant(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	svc := application.NewListRunsService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), newFakeVisibilityChecker())

	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, testWorkspaceID, "member-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without a visibility grant, got: %v", err)
	}
}
