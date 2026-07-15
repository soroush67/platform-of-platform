package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
)

const testOrgID = "org-1"
const testProjectID = "project-1"
const testWorkspaceID = "ws-1"

func newTriggerRunService() (*application.TriggerRunService, *fakeRunRepo, *fakeWorkspaceLocker, *fakeWorkspaceChecker, *fakeScopedPermissionChecker, *fakeOrganizationChecker) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	workspaceChecker := newFakeWorkspaceChecker()
	permChecker := newFakeScopedPermissionChecker()
	orgChecker := newFakeOrganizationChecker()
	svc := application.NewTriggerRunService(runRepo, locker, workspaceChecker, permChecker, orgChecker)
	return svc, runRepo, locker, workspaceChecker, permChecker, orgChecker
}

func TestTriggerRunService_UnknownWorkspaceGetsNotFoundBeforePermissionCheck(t *testing.T) {
	svc, _, _, _, _, _ := newTriggerRunService()

	// Deliberately no grant on permChecker either - if the service
	// checked permission before workspace existence, this would come
	// back ErrForbidden instead, the wrong ordering.
	_, err := svc.Execute(context.Background(), application.TriggerRunInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: testWorkspaceID, RequestingUserID: "user-1",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got: %v", err)
	}
}

func TestTriggerRunService_RequiresWorkspaceApply(t *testing.T) {
	svc, _, _, workspaceChecker, _, _ := newTriggerRunService()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)

	_, err := svc.Execute(context.Background(), application.TriggerRunInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: testWorkspaceID, RequestingUserID: "user-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without workspace:apply, got: %v", err)
	}
}

func TestTriggerRunService_ArchivedOrgRejected(t *testing.T) {
	svc, _, _, workspaceChecker, permChecker, orgChecker := newTriggerRunService()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	permChecker.grant(testOrgID, "user-1", "workspace:apply")
	orgChecker.archive(testOrgID)

	_, err := svc.Execute(context.Background(), application.TriggerRunInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: testWorkspaceID, RequestingUserID: "user-1",
	})
	if !errors.Is(err, domain.ErrOrganizationArchived) {
		t.Fatalf("expected ErrOrganizationArchived, got: %v", err)
	}
}

func TestTriggerRunService_AlreadyLockedWorkspaceRejected(t *testing.T) {
	svc, _, locker, workspaceChecker, permChecker, _ := newTriggerRunService()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	permChecker.grant(testOrgID, "user-1", "workspace:apply")
	locked, err := locker.TryLock(context.Background(), testOrgID, testWorkspaceID, "some-other-run")
	if err != nil || !locked {
		t.Fatalf("test setup: expected the lock to be acquired, got locked=%v err=%v", locked, err)
	}

	_, err = svc.Execute(context.Background(), application.TriggerRunInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: testWorkspaceID, RequestingUserID: "user-1",
	})
	if !errors.Is(err, domain.ErrWorkspaceLocked) {
		t.Fatalf("expected ErrWorkspaceLocked, got: %v", err)
	}
}

func TestTriggerRunService_SucceedsAndLocksTheWorkspace(t *testing.T) {
	svc, runRepo, locker, workspaceChecker, permChecker, _ := newTriggerRunService()
	workspaceChecker.add(testOrgID, testProjectID, testWorkspaceID)
	permChecker.grant(testOrgID, "user-1", "workspace:apply")

	run, err := svc.Execute(context.Background(), application.TriggerRunInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: testWorkspaceID, RequestingUserID: "user-1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if run.Status != domain.RunStatusQueued {
		t.Errorf("expected a new run to start queued, got %q", run.Status)
	}
	if _, err := runRepo.GetByID(context.Background(), testOrgID, run.ID); err != nil {
		t.Errorf("expected the run to be persisted, got: %v", err)
	}
	if !locker.isLocked(testWorkspaceID) {
		t.Error("expected TriggerRunService to lock the workspace")
	}
}
