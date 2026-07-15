package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/rbac/application"
	"platform-of-platform/internal/rbac/domain"
)

const testOrgID = "org-1"

func TestCreateRoleService_RejectsUnknownPermission(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	svc := application.NewCreateRoleService(newFakeRoleRepo(), membership, perm)

	_, err := svc.Execute(context.Background(), application.CreateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Name: "custom", Permissions: []string{"not:a:real:permission"},
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unknown permission, got: %v", err)
	}
}

func TestCreateRoleService_RequiresOrganizationManage(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	svc := application.NewCreateRoleService(newFakeRoleRepo(), membership, newFakePermissionChecker())

	_, err := svc.Execute(context.Background(), application.CreateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", Name: "custom", Permissions: []string{"workspace:read"},
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestCreateRoleService_ComposesOnlyExistingPermissions(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	repo := newFakeRoleRepo()
	svc := application.NewCreateRoleService(repo, membership, perm)

	role, err := svc.Execute(context.Background(), application.CreateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Name: "deployer",
		Permissions: []string{"workspace:apply", "workspace:read"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(role.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %v", role.Permissions)
	}
	if role.OrganizationID == nil || *role.OrganizationID != testOrgID {
		t.Error("expected the role to be scoped to this organization, not a built-in")
	}
}

func TestListRolesService_MembershipGatedOnly(t *testing.T) {
	membership := newFakeMembershipChecker()
	repo := newFakeRoleRepo()
	builtinOwner, _ := domain.NewRole(testOrgID, "owner", []domain.Permission{domain.PermissionOrganizationManage})
	builtinOwner.OrganizationID = nil // simulate a real built-in: organization_id IS NULL
	repo.put(builtinOwner)
	svc := application.NewListRolesService(repo, membership)

	if _, err := svc.Execute(context.Background(), testOrgID, "stranger"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-member, got: %v", err)
	}

	membership.add(testOrgID, "member-1")
	roles, err := svc.Execute(context.Background(), testOrgID, "member-1")
	if err != nil {
		t.Fatalf("expected a member to list roles without any extra RBAC grant, got: %v", err)
	}
	if len(roles) != 1 {
		t.Errorf("expected 1 visible role, got %d", len(roles))
	}
}
