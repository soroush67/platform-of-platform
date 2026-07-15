package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

const testOrgID = "org-1"
const testProjectID = "project-1"

func TestCreateEnvironmentService_RequiresOrganizationManage(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	svc := application.NewCreateEnvironmentService(newFakeEnvironmentRepo(), membership, newFakePermissionChecker(), projectChecker)

	_, err := svc.Execute(context.Background(), application.CreateEnvironmentInput{
		OrganizationID: testOrgID, ProjectID: testProjectID, RequestingUserID: "member-1", Name: "dev",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestCreateEnvironmentService_UnknownProjectGetsNotFoundBeforePermissionCheck(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	// Deliberately NOT granting organization:manage - if the service
	// checked permission before project existence, this would come back
	// ErrForbidden instead, the wrong ordering.
	svc := application.NewCreateEnvironmentService(newFakeEnvironmentRepo(), membership, newFakePermissionChecker(), newFakeProjectChecker())

	_, err := svc.Execute(context.Background(), application.CreateEnvironmentInput{
		OrganizationID: testOrgID, ProjectID: "nonexistent", RequestingUserID: "admin-1", Name: "dev",
	})
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got: %v", err)
	}
}

func TestGetEnvironmentService_RejectsEnvironmentFromAnotherProject(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	envRepo := newFakeEnvironmentRepo()
	env, _ := domain.NewEnvironment(testOrgID, "a-different-project", "dev", 0, false)
	_ = envRepo.Create(context.Background(), env)

	svc := application.NewGetEnvironmentService(envRepo, membership, projectChecker)
	_, err := svc.Execute(context.Background(), testOrgID, testProjectID, env.ID, "member-1")
	if !errors.Is(err, domain.ErrEnvironmentNotFound) {
		t.Fatalf("expected ErrEnvironmentNotFound for an environment under a different project, got: %v", err)
	}
}

func TestListEnvironmentsService_ScopedToProject(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add(testOrgID, testProjectID)
	envRepo := newFakeEnvironmentRepo()
	inProject, _ := domain.NewEnvironment(testOrgID, testProjectID, "dev", 0, false)
	otherProject, _ := domain.NewEnvironment(testOrgID, "other-project", "staging", 1, false)
	_ = envRepo.Create(context.Background(), inProject)
	_ = envRepo.Create(context.Background(), otherProject)

	svc := application.NewListEnvironmentsService(envRepo, membership, projectChecker)
	got, err := svc.Execute(context.Background(), testOrgID, testProjectID, "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != inProject.ID {
		t.Errorf("expected exactly the one environment in this project, got %+v", got)
	}
}
