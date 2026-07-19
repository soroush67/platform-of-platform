package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/variables/application"
	"platform-of-platform/internal/variables/domain"
)

func mustVariable(t *testing.T, scopeType domain.ScopeType, scopeID, key string) *domain.Variable {
	t.Helper()
	v, err := domain.NewVariable(testOrgID, scopeType, scopeID, key, domain.CategoryEnvVar, domain.SensitivityPlain, "original")
	if err != nil {
		t.Fatalf("NewVariable: %v", err)
	}
	return v
}

func TestUpdateVariableService_NonMemberGetsNotFound(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	svc := application.NewUpdateVariableService(repo, membership, newFakePermissionChecker())

	_, err := svc.Execute(context.Background(), application.UpdateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "stranger", VariableID: "var-1",
		Value: "new", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain,
	})
	if !errors.Is(err, domain.ErrVariableNotFound) {
		t.Fatalf("expected ErrVariableNotFound for a non-member, got: %v", err)
	}
}

func TestUpdateVariableService_WorkspaceScopedVariableRequiresWorkspaceManage(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	v := mustVariable(t, domain.ScopeTypeWorkspace, testWorkspaceID, "FOO")
	repo.put(v)
	svc := application.NewUpdateVariableService(repo, membership, permChecker)

	_, err := svc.Execute(context.Background(), application.UpdateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", VariableID: v.ID,
		Value: "new", Category: domain.CategoryEnvVar, Sensitivity: domain.SensitivityPlain,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without workspace:manage specifically, got: %v", err)
	}
}

func TestUpdateVariableService_RejectsInvalidCategory(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	v := mustVariable(t, domain.ScopeTypeOrganization, testOrgID, "FOO")
	repo.put(v)
	svc := application.NewUpdateVariableService(repo, membership, permChecker)

	_, err := svc.Execute(context.Background(), application.UpdateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", VariableID: v.ID,
		Value: "new", Category: "not-a-real-category", Sensitivity: domain.SensitivityPlain,
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an invalid category, got: %v", err)
	}
}

func TestUpdateVariableService_Succeeds(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	v := mustVariable(t, domain.ScopeTypeOrganization, testOrgID, "FOO")
	repo.put(v)
	svc := application.NewUpdateVariableService(repo, membership, permChecker)

	got, err := svc.Execute(context.Background(), application.UpdateVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", VariableID: v.ID,
		Value: "updated", Category: domain.CategoryEngineVar, Sensitivity: domain.SensitivitySensitive,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Value != "updated" || got.Category != domain.CategoryEngineVar || got.Sensitivity != domain.SensitivitySensitive {
		t.Errorf("expected the update to apply, got %+v", got)
	}
	stored, _ := repo.GetByID(context.Background(), testOrgID, v.ID)
	if stored.Value != "updated" {
		t.Errorf("expected the update to be persisted, got %+v", stored)
	}
}

func TestDeleteVariableService_NonMemberGetsNotFound(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	svc := application.NewDeleteVariableService(repo, membership, newFakePermissionChecker())

	err := svc.Execute(context.Background(), application.DeleteVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "stranger", VariableID: "var-1",
	})
	if !errors.Is(err, domain.ErrVariableNotFound) {
		t.Fatalf("expected ErrVariableNotFound for a non-member, got: %v", err)
	}
}

func TestDeleteVariableService_RequiresPermissionForTheVariablesScope(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	v := mustVariable(t, domain.ScopeTypeOrganization, testOrgID, "FOO")
	repo.put(v)
	svc := application.NewDeleteVariableService(repo, membership, newFakePermissionChecker())

	err := svc.Execute(context.Background(), application.DeleteVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", VariableID: v.ID,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestDeleteVariableService_Succeeds(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	v := mustVariable(t, domain.ScopeTypeOrganization, testOrgID, "FOO")
	repo.put(v)
	svc := application.NewDeleteVariableService(repo, membership, permChecker)

	if err := svc.Execute(context.Background(), application.DeleteVariableInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", VariableID: v.ID,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), testOrgID, v.ID); !errors.Is(err, domain.ErrVariableNotFound) {
		t.Errorf("expected the variable to be gone, got: %v", err)
	}
}

func TestListVariablesService_NonMemberGetsScopeNotFound(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	svc := application.NewListVariablesService(repo, membership, newFakePermissionChecker(), newFakeVisibilityChecker(), newFakeWorkspaceChecker(), newFakeEnvironmentProjectResolver())

	_, err := svc.Execute(context.Background(), testOrgID, "stranger", domain.ScopeTypeOrganization, testOrgID)
	if !errors.Is(err, domain.ErrScopeNotFound) {
		t.Fatalf("expected ErrScopeNotFound for a non-member, got: %v", err)
	}
}

func TestListVariablesService_OrganizationScopeStaysMembershipOnly(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	v := mustVariable(t, domain.ScopeTypeOrganization, testOrgID, "FOO")
	repo.put(v)
	// No organization:manage, no project-scope grant at all - org-scoped
	// Variables stay visible to every member regardless (genuinely
	// org-wide config, not Project-specific - see project_visibility.go).
	svc := application.NewListVariablesService(repo, membership, newFakePermissionChecker(), newFakeVisibilityChecker(), newFakeWorkspaceChecker(), newFakeEnvironmentProjectResolver())

	got, err := svc.Execute(context.Background(), testOrgID, "member-1", domain.ScopeTypeOrganization, testOrgID)
	if err != nil {
		t.Fatalf("expected org-scoped Variables to stay membership-only, got: %v", err)
	}
	if len(got) != 1 || got[0].ID != v.ID {
		t.Errorf("expected the org-scoped variable, got %+v", got)
	}
}

func TestListVariablesService_ScopedToExactly(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	inScope := mustVariable(t, domain.ScopeTypeProject, testProjectID, "A")
	otherScope := mustVariable(t, domain.ScopeTypeProject, "other-project", "B")
	repo.put(inScope)
	repo.put(otherScope)
	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(testOrgID, "member-1", "project:read", "project", testProjectID)
	svc := application.NewListVariablesService(repo, membership, newFakePermissionChecker(), visibilityChecker, newFakeWorkspaceChecker(), newFakeEnvironmentProjectResolver())

	got, err := svc.Execute(context.Background(), testOrgID, "member-1", domain.ScopeTypeProject, testProjectID)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != inScope.ID {
		t.Errorf("expected exactly the one variable in this scope, got %+v", got)
	}
}

func TestListVariablesService_ProjectScopeForbiddenWithoutGrant(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	v := mustVariable(t, domain.ScopeTypeProject, testProjectID, "A")
	repo.put(v)
	svc := application.NewListVariablesService(repo, membership, newFakePermissionChecker(), newFakeVisibilityChecker(), newFakeWorkspaceChecker(), newFakeEnvironmentProjectResolver())

	_, err := svc.Execute(context.Background(), testOrgID, "member-1", domain.ScopeTypeProject, testProjectID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without a project-scope grant, got: %v", err)
	}
}

func TestListVariablesService_WorkspaceScopeResolvesProjectViaWorkspaceChecker(t *testing.T) {
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	v := mustVariable(t, domain.ScopeTypeWorkspace, testWorkspaceID, "A")
	repo.put(v)
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add(testOrgID, testWorkspaceID, testProjectID, nil)
	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(testOrgID, "member-1", "project:read", "project", testProjectID)
	svc := application.NewListVariablesService(repo, membership, newFakePermissionChecker(), visibilityChecker, workspaceChecker, newFakeEnvironmentProjectResolver())

	got, err := svc.Execute(context.Background(), testOrgID, "member-1", domain.ScopeTypeWorkspace, testWorkspaceID)
	if err != nil {
		t.Fatalf("expected a project-scope grant on the workspace's own project to allow it, got: %v", err)
	}
	if len(got) != 1 || got[0].ID != v.ID {
		t.Errorf("expected the workspace-scoped variable, got %+v", got)
	}
}
