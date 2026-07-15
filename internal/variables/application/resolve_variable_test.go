package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/variables/application"
	"platform-of-platform/internal/variables/domain"
)

func setupResolveService(t *testing.T) (*application.ResolveVariableService, *fakeVariableRepo, *fakeMembershipChecker, *fakeWorkspaceChecker) {
	t.Helper()
	repo := newFakeVariableRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	workspaceChecker := newFakeWorkspaceChecker()
	environmentID := "env-1"
	workspaceChecker.add(testOrgID, testWorkspaceID, testProjectID, &environmentID)
	svc := application.NewResolveVariableService(repo, membership, workspaceChecker)
	return svc, repo, membership, workspaceChecker
}

func TestResolveVariableService_UnknownWorkspaceGetsScopeNotFound(t *testing.T) {
	svc, _, _, _ := setupResolveService(t)

	_, err := svc.Execute(context.Background(), testOrgID, "nonexistent-ws", "FOO", "member-1")
	if !errors.Is(err, domain.ErrScopeNotFound) {
		t.Fatalf("expected ErrScopeNotFound for an unknown workspace, got: %v", err)
	}
}

func TestResolveVariableService_WorkspaceScopeWinsOverEverythingElse(t *testing.T) {
	svc, repo, _, _ := setupResolveService(t)
	repo.put(mustVariable(t, domain.ScopeTypeWorkspace, testWorkspaceID, "FOO"))
	orgVar := mustVariable(t, domain.ScopeTypeOrganization, testOrgID, "FOO")
	orgVar.Value = "should not win"
	repo.put(orgVar)

	got, err := svc.Execute(context.Background(), testOrgID, testWorkspaceID, "FOO", "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.ScopeType != domain.ScopeTypeWorkspace {
		t.Errorf("expected the workspace-scoped variable to win, got scope %q", got.ScopeType)
	}
}

func TestResolveVariableService_FallsThroughToProjectWhenNoEnvironmentMatch(t *testing.T) {
	svc, repo, _, _ := setupResolveService(t)
	repo.put(mustVariable(t, domain.ScopeTypeProject, testProjectID, "FOO"))

	got, err := svc.Execute(context.Background(), testOrgID, testWorkspaceID, "FOO", "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.ScopeType != domain.ScopeTypeProject {
		t.Errorf("expected the cascade to fall through workspace and environment down to project, got scope %q", got.ScopeType)
	}
}

func TestResolveVariableService_NoMatchAnywhereReturnsVariableNotFound(t *testing.T) {
	svc, _, _, _ := setupResolveService(t)

	_, err := svc.Execute(context.Background(), testOrgID, testWorkspaceID, "NOPE", "member-1")
	if !errors.Is(err, domain.ErrVariableNotFound) {
		t.Fatalf("expected ErrVariableNotFound, got: %v", err)
	}
}

func TestResolveVariableService_ResolveValueTranslatesNotFoundToFalseNotError(t *testing.T) {
	svc, _, _, _ := setupResolveService(t)

	value, found, err := svc.ResolveValue(context.Background(), testOrgID, testWorkspaceID, "NOPE", "member-1")
	if err != nil {
		t.Fatalf("expected ResolveValue to translate ErrVariableNotFound into (\"\", false, nil), got err: %v", err)
	}
	if found || value != "" {
		t.Errorf("expected not found, got value=%q found=%v", value, found)
	}
}
