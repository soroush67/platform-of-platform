package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/rbac/application"
	"platform-of-platform/internal/rbac/domain"
)

func setupRoleBindingService(t *testing.T) (*application.CreateRoleBindingService, *fakeMembershipChecker, *fakePermissionChecker, *fakeRoleRepo, *fakeResourceChecker, *domain.Role) {
	t.Helper()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	roleRepo := newFakeRoleRepo()
	role, err := domain.NewRole(testOrgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	if err != nil {
		t.Fatalf("NewRole: %v", err)
	}
	roleRepo.put(role)
	resourceChecker := newFakeResourceChecker()
	bindingRepo := newFakeRoleBindingRepo()

	svc := application.NewCreateRoleBindingService(roleRepo, bindingRepo, membership, perm, resourceChecker, resourceChecker, resourceChecker, resourceChecker)
	return svc, membership, perm, roleRepo, resourceChecker, role
}

func TestCreateRoleBindingService_DefaultsToAllowEffect(t *testing.T) {
	svc, membership, _, _, _, role := setupRoleBindingService(t)
	membership.add(testOrgID, "target-user")

	binding, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeUser, SubjectID: "target-user",
		ScopeType: domain.ScopeTypeOrganization, ScopeID: testOrgID,
		// Effect left empty on purpose.
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if binding.Effect != domain.EffectAllow {
		t.Errorf("expected effect to default to %q, got %q", domain.EffectAllow, binding.Effect)
	}
}

func TestCreateRoleBindingService_RejectsInvalidEffect(t *testing.T) {
	svc, membership, _, _, _, role := setupRoleBindingService(t)
	membership.add(testOrgID, "target-user")

	_, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeUser, SubjectID: "target-user",
		ScopeType: domain.ScopeTypeOrganization, ScopeID: testOrgID,
		Effect: "maybe",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an invalid effect, got: %v", err)
	}
}

func TestCreateRoleBindingService_UserSubjectMustBeOrgMember(t *testing.T) {
	svc, _, _, _, _, role := setupRoleBindingService(t)

	_, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeUser, SubjectID: "not-a-member",
		ScopeType: domain.ScopeTypeOrganization, ScopeID: testOrgID,
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for a non-member user subject, got: %v", err)
	}
}

func TestCreateRoleBindingService_TeamSubjectMustExist(t *testing.T) {
	svc, _, _, _, resourceChecker, role := setupRoleBindingService(t)

	_, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeTeam, SubjectID: "team-1",
		ScopeType: domain.ScopeTypeOrganization, ScopeID: testOrgID,
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unknown team, got: %v", err)
	}

	resourceChecker.add(testOrgID, "team-1")
	if _, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeTeam, SubjectID: "team-1",
		ScopeType: domain.ScopeTypeOrganization, ScopeID: testOrgID,
	}); err != nil {
		t.Fatalf("expected success once the team exists, got: %v", err)
	}
}

func TestCreateRoleBindingService_WorkspaceScopeMustExist(t *testing.T) {
	svc, membership, _, _, resourceChecker, role := setupRoleBindingService(t)
	membership.add(testOrgID, "target-user")

	_, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeUser, SubjectID: "target-user",
		ScopeType: domain.ScopeTypeWorkspace, ScopeID: "ws-1",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for a nonexistent workspace scope, got: %v", err)
	}

	resourceChecker.add(testOrgID, "ws-1")
	if _, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		SubjectType: domain.SubjectTypeUser, SubjectID: "target-user",
		ScopeType: domain.ScopeTypeWorkspace, ScopeID: "ws-1",
	}); err != nil {
		t.Fatalf("expected success once the workspace exists, got: %v", err)
	}
}

func TestCreateRoleBindingService_RoleFromAnotherOrgRejected(t *testing.T) {
	svc, membership, _, roleRepo, _, _ := setupRoleBindingService(t)
	membership.add(testOrgID, "target-user")

	otherOrgID := "org-2"
	foreignRole, _ := domain.NewRole(otherOrgID, "foreign-role", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(foreignRole)

	_, err := svc.Execute(context.Background(), application.CreateRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: foreignRole.ID,
		SubjectType: domain.SubjectTypeUser, SubjectID: "target-user",
		ScopeType: domain.ScopeTypeOrganization, ScopeID: testOrgID,
	})
	if !errors.Is(err, domain.ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound for a role belonging to a different organization, got: %v", err)
	}
}

func setupListRoleBindingsService(t *testing.T) (*application.ListRoleBindingsService, *fakeMembershipChecker, *fakeRoleRepo, *fakeRoleBindingRepo, *fakeUserReader, *fakeNameReader) {
	t.Helper()
	membership := newFakeMembershipChecker()
	roleRepo := newFakeRoleRepo()
	bindingRepo := newFakeRoleBindingRepo()
	userReader := newFakeUserReader()
	nameReader := newFakeNameReader()
	svc := application.NewListRoleBindingsService(bindingRepo, membership, roleRepo, userReader, nameReader, nameReader, nameReader, nameReader)
	return svc, membership, roleRepo, bindingRepo, userReader, nameReader
}

func TestListRoleBindingsService_RequiresMembership(t *testing.T) {
	svc, _, _, _, _, _ := setupListRoleBindingsService(t)

	if _, err := svc.Execute(context.Background(), testOrgID, "", "stranger"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-member, got: %v", err)
	}
}

func TestListRoleBindingsService_ResolvesDisplayNames(t *testing.T) {
	svc, membership, roleRepo, bindingRepo, userReader, nameReader := setupListRoleBindingsService(t)
	membership.add(testOrgID, "requester")

	role, _ := domain.NewRole(testOrgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(role)
	userReader.set("user-1", "alice", "alice@example.com")
	nameReader.set(testOrgID, "project-1", "My Project")

	binding := domain.NewRoleBinding(testOrgID, role.ID, domain.SubjectTypeUser, "user-1", domain.ScopeTypeProject, "project-1", domain.EffectAllow)
	if err := bindingRepo.Create(context.Background(), binding); err != nil {
		t.Fatalf("Create binding: %v", err)
	}

	got, err := svc.Execute(context.Background(), testOrgID, "", "requester")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(got))
	}
	summary := got[0]
	if summary.RoleName != "deployer" || summary.SubjectName != "alice" || summary.ScopeName != "My Project" {
		t.Errorf("expected resolved role/subject/scope names, got %+v", summary)
	}
}

func TestListRoleBindingsService_UnresolvableNameStaysEmptyNotError(t *testing.T) {
	svc, membership, roleRepo, bindingRepo, _, _ := setupListRoleBindingsService(t)
	membership.add(testOrgID, "requester")

	role, _ := domain.NewRole(testOrgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(role)

	// A team subject with no matching fakeNameReader entry - a
	// since-deleted Team, or one this fake just never had set - must
	// show up as an empty name, not fail the whole list.
	binding := domain.NewRoleBinding(testOrgID, role.ID, domain.SubjectTypeTeam, "team-gone", domain.ScopeTypeOrganization, testOrgID, domain.EffectAllow)
	if err := bindingRepo.Create(context.Background(), binding); err != nil {
		t.Fatalf("Create binding: %v", err)
	}

	got, err := svc.Execute(context.Background(), testOrgID, "", "requester")
	if err != nil {
		t.Fatalf("expected no error even though the team can't be resolved, got: %v", err)
	}
	if len(got) != 1 || got[0].SubjectName != "" {
		t.Errorf("expected an empty (not failing) subject name, got %+v", got)
	}
}
