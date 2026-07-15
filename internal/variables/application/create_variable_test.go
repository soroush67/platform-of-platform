package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/variables/application"
	"platform-of-platform/internal/variables/domain"
)

const testOrgID = "org-1"
const testProjectID = "project-1"
const testWorkspaceID = "ws-1"

func newCreateVariableService() (*application.CreateVariableService, *fakeVariableRepo, *fakeMembershipChecker, *fakeProjectChecker, *fakeEnvironmentChecker, *fakeWorkspaceChecker, *fakePermissionChecker, *fakeOrganizationChecker, *fakeSecretMountChecker) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	projectChecker := newFakeProjectChecker()
	envChecker := newFakeEnvironmentChecker()
	workspaceChecker := newFakeWorkspaceChecker()
	permChecker := newFakePermissionChecker()
	orgChecker := newFakeOrganizationChecker()
	secretMountChecker := newFakeSecretMountChecker()
	svc := application.NewCreateVariableService(repo, membership, projectChecker, envChecker, workspaceChecker, permChecker, orgChecker, secretMountChecker)
	return svc, repo, membership, projectChecker, envChecker, workspaceChecker, permChecker, orgChecker, secretMountChecker
}

func TestCreateVariableService_NonMemberGetsScopeNotFoundBeforeAnyOtherCheck(t *testing.T) {
	svc, _, _, _, _, _, _, _, _ := newCreateVariableService()

	// Deliberately no membership, no scope registered, no permission
	// granted - a non-member must get ErrScopeNotFound, not whatever
	// error would otherwise surface first.
	_, err := svc.Execute(context.Background(), application.CreateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "stranger", ScopeType: domain.ScopeTypeOrganization,
		ScopeID: testOrgID, Key: "FOO", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain, Value: "bar",
	})
	if !errors.Is(err, domain.ErrScopeNotFound) {
		t.Fatalf("expected ErrScopeNotFound for a non-member, got: %v", err)
	}
}

func TestCreateVariableService_UnknownProjectScopeRejected(t *testing.T) {
	svc, _, membership, _, _, _, _, _, _ := newCreateVariableService()
	membership.add(testOrgID, "member-1")

	_, err := svc.Execute(context.Background(), application.CreateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", ScopeType: domain.ScopeTypeProject,
		ScopeID: "nonexistent-project", Key: "FOO", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain, Value: "bar",
	})
	if !errors.Is(err, domain.ErrScopeNotFound) {
		t.Fatalf("expected ErrScopeNotFound for an unknown project scope, got: %v", err)
	}
}

func TestCreateVariableService_WorkspaceScopeRequiresWorkspaceManageNotOrganizationManage(t *testing.T) {
	svc, _, membership, _, _, workspaceChecker, permChecker, _, _ := newCreateVariableService()
	membership.add(testOrgID, "member-1")
	workspaceChecker.add(testOrgID, testWorkspaceID, testProjectID, nil)
	// Grant only organization:manage, not workspace:manage - the
	// workspace-scope tier must NOT accept the broader org tier as a
	// substitute (each scope has its own gate).
	permChecker.grant(testOrgID, "member-1", "organization:manage")

	_, err := svc.Execute(context.Background(), application.CreateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", ScopeType: domain.ScopeTypeWorkspace,
		ScopeID: testWorkspaceID, Key: "FOO", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain, Value: "bar",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without workspace:manage specifically, got: %v", err)
	}
}

func TestCreateVariableService_ArchivedOrgRejected(t *testing.T) {
	svc, _, membership, _, _, _, permChecker, orgChecker, _ := newCreateVariableService()
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	orgChecker.archive(testOrgID)

	_, err := svc.Execute(context.Background(), application.CreateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", ScopeType: domain.ScopeTypeOrganization,
		ScopeID: testOrgID, Key: "FOO", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain, Value: "bar",
	})
	if !errors.Is(err, domain.ErrOrganizationArchived) {
		t.Fatalf("expected ErrOrganizationArchived, got: %v", err)
	}
}

func TestCreateVariableService_Succeeds(t *testing.T) {
	svc, repo, membership, _, _, _, permChecker, _, _ := newCreateVariableService()
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")

	v, err := svc.Execute(context.Background(), application.CreateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", ScopeType: domain.ScopeTypeOrganization,
		ScopeID: testOrgID, Key: "FOO", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain, Value: "bar",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), testOrgID, v.ID); err != nil {
		t.Errorf("expected the variable to be persisted, got: %v", err)
	}
}
