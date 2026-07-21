package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

func TestDeleteWorkspaceService_RequiresWorkspaceDelete(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	wsRepo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace(testOrgID, testProjectID, nil, "ws", domain.ExecutionEngineTerraform)
	wsRepo.put(ws)

	permChecker := newFakePermissionChecker()
	svc := application.NewDeleteWorkspaceService(wsRepo, wsRepo, membership, permChecker, projectChecker)

	err := svc.Execute(context.Background(), application.DeleteWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: ws.ID, RequestingUserID: "member-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without workspace:delete, got: %v", err)
	}

	permChecker.grant(testOrgID, "member-1", "workspace:delete")
	if err := svc.Execute(context.Background(), application.DeleteWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: ws.ID, RequestingUserID: "member-1",
	}); err != nil {
		t.Fatalf("expected deletion to succeed once granted, got: %v", err)
	}
	if _, err := wsRepo.GetByID(context.Background(), testOrgID, ws.ID); !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected workspace to be gone after Purge, got: %v", err)
	}
}

func TestDeleteWorkspaceService_NonMemberGetsProjectNotFound(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "real-member")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	wsRepo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace(testOrgID, testProjectID, nil, "ws", domain.ExecutionEngineTerraform)
	wsRepo.put(ws)

	svc := application.NewDeleteWorkspaceService(wsRepo, wsRepo, membership, newFakePermissionChecker(), projectChecker)
	err := svc.Execute(context.Background(), application.DeleteWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, WorkspaceID: ws.ID, RequestingUserID: "stranger",
	})
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound for a non-member, got: %v", err)
	}
}

func TestDeleteWorkspaceService_WrongProjectGetsWorkspaceNotFound(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "workspace:delete")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	projectChecker.add(testOrgID, "a-different-project")
	wsRepo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace(testOrgID, testProjectID, nil, "ws", domain.ExecutionEngineTerraform)
	wsRepo.put(ws)

	svc := application.NewDeleteWorkspaceService(wsRepo, wsRepo, membership, permChecker, projectChecker)
	err := svc.Execute(context.Background(), application.DeleteWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: "a-different-project", WorkspaceID: ws.ID, RequestingUserID: "member-1",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a workspace id belonging to a different project, got: %v", err)
	}
}
