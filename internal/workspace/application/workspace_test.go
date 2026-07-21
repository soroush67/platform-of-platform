package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

func TestCreateWorkspaceService_RequiresWorkspaceManage(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	svc := application.NewCreateWorkspaceService(newFakeWorkspaceRepo(), newFakeEnvironmentRepo(), membership, newFakePermissionChecker(), projectChecker, newFakeOrganizationChecker())

	_, err := svc.Execute(context.Background(), application.CreateWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, RequestingUserID: "member-1", Name: "ws", ExecutionEngine: domain.ExecutionEngineTerraform,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without workspace:manage, got: %v", err)
	}
}

func TestCreateWorkspaceService_ArchivedOrgRejected(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "workspace:manage")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	orgChecker := newFakeOrganizationChecker()
	orgChecker.archive(testOrgID)

	svc := application.NewCreateWorkspaceService(newFakeWorkspaceRepo(), newFakeEnvironmentRepo(), membership, permChecker, projectChecker, orgChecker)
	_, err := svc.Execute(context.Background(), application.CreateWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, RequestingUserID: "member-1", Name: "ws", ExecutionEngine: domain.ExecutionEngineTerraform,
	})
	if !errors.Is(err, domain.ErrOrganizationArchived) {
		t.Fatalf("expected ErrOrganizationArchived, got: %v", err)
	}
}

func TestCreateWorkspaceService_EnvironmentMustBelongToSameProject(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "workspace:manage")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	envRepo := newFakeEnvironmentRepo()
	wrongProjectEnv, _ := domain.NewEnvironment(testOrgID, "a-different-project", "dev", 0, false)
	_ = envRepo.Create(context.Background(), wrongProjectEnv)

	svc := application.NewCreateWorkspaceService(newFakeWorkspaceRepo(), envRepo, membership, permChecker, projectChecker, newFakeOrganizationChecker())
	_, err := svc.Execute(context.Background(), application.CreateWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, RequestingUserID: "member-1", Name: "ws",
		ExecutionEngine: domain.ExecutionEngineTerraform, EnvironmentID: &wrongProjectEnv.ID,
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an environment belonging to a different project, got: %v", err)
	}
}

func TestCreateWorkspaceService_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "workspace:manage")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	repo := newFakeWorkspaceRepo()

	svc := application.NewCreateWorkspaceService(repo, newFakeEnvironmentRepo(), membership, permChecker, projectChecker, newFakeOrganizationChecker())
	ws, err := svc.Execute(context.Background(), application.CreateWorkspaceInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, RequestingUserID: "member-1", Name: "ws", ExecutionEngine: domain.ExecutionEngineTerraform,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), testOrgID, ws.ID); err != nil {
		t.Errorf("expected the workspace to be persisted, got: %v", err)
	}
}

func TestGetWorkspaceService_RejectsWorkspaceFromAnotherProject(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	repo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace(testOrgID, "a-different-project", nil, "ws", domain.ExecutionEngineTerraform)
	repo.put(ws)

	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	svc := application.NewGetWorkspaceService(repo, membership, projectChecker, permChecker, newFakeVisibilityChecker())
	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, ws.ID, "member-1")
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound for a workspace under a different project, got: %v", err)
	}
}

func TestGetWorkspaceService_RequiresVisibilityGrant(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	repo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace(testOrgID, testProjectID, nil, "ws", domain.ExecutionEngineTerraform)
	repo.put(ws)

	// No organization:manage and no project/workspace-scope grant at
	// all - a plain org member no longer sees every Workspace by
	// default (the whole point of this session's per-project visibility
	// change).
	svc := application.NewGetWorkspaceService(repo, membership, projectChecker, newFakePermissionChecker(), newFakeVisibilityChecker())
	if _, err := svc.Execute(context.Background(), testOrgID, testProjectID, ws.ID, "member-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without a visibility grant, got: %v", err)
	}

	// A workspace-scope grant alone (no project-scope grant) is enough -
	// handing out one Workspace without the whole Project.
	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(testOrgID, "member-1", "workspace:read", "workspace", ws.ID)
	svc = application.NewGetWorkspaceService(repo, membership, projectChecker, newFakePermissionChecker(), visibilityChecker)
	if _, err := svc.Execute(context.Background(), testOrgID, testProjectID, ws.ID, "member-1"); err != nil {
		t.Fatalf("expected a workspace-scope grant to allow reading it, got: %v", err)
	}
}

func TestListWorkspacesService_ScopedToProject(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	repo := newFakeWorkspaceRepo()
	inProject, _ := domain.NewWorkspace(testOrgID, testProjectID, nil, "ws-a", domain.ExecutionEngineTerraform)
	otherProject, _ := domain.NewWorkspace(testOrgID, "other-project", nil, "ws-b", domain.ExecutionEngineTerraform)
	repo.put(inProject)
	repo.put(otherProject)

	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(testOrgID, "member-1", "project:read", "project", testProjectID)
	svc := application.NewListWorkspacesService(repo, membership, projectChecker, newFakePermissionChecker(), visibilityChecker)
	got, err := svc.Execute(context.Background(), testOrgID, testProjectID, "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != inProject.ID {
		t.Errorf("expected exactly the one workspace in this project, got %+v", got)
	}
}

func TestListWorkspacesService_ForbiddenWithoutVisibilityGrant(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	repo := newFakeWorkspaceRepo()

	svc := application.NewListWorkspacesService(repo, membership, projectChecker, newFakePermissionChecker(), newFakeVisibilityChecker())
	if _, err := svc.Execute(context.Background(), testOrgID, testProjectID, "member-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without a visibility grant, got: %v", err)
	}
}
