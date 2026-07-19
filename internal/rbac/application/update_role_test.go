package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/rbac/application"
	"platform-of-platform/internal/rbac/domain"
)

func TestUpdateRoleService_RequiresOrganizationManage(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	repo := newFakeRoleRepo()
	role, _ := domain.NewRole(testOrgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	repo.put(role)
	svc := application.NewUpdateRoleService(repo, membership, newFakePermissionChecker())

	_, err := svc.Execute(context.Background(), application.UpdateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", RoleID: role.ID,
		Permissions: []string{"workspace:read"},
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestUpdateRoleService_RejectsBuiltinRole(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	repo := newFakeRoleRepo()
	builtin, _ := domain.NewRole(testOrgID, "owner", []domain.Permission{domain.PermissionOrganizationManage})
	builtin.OrganizationID = nil // simulate a real built-in
	repo.put(builtin)
	svc := application.NewUpdateRoleService(repo, membership, perm)

	_, err := svc.Execute(context.Background(), application.UpdateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: builtin.ID,
		Permissions: []string{"workspace:read"},
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when editing a built-in role, got: %v", err)
	}
}

func TestUpdateRoleService_UpdatesCustomRoleInPlace(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	repo := newFakeRoleRepo()
	role, _ := domain.NewRole(testOrgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	repo.put(role)
	svc := application.NewUpdateRoleService(repo, membership, perm)

	updated, err := svc.Execute(context.Background(), application.UpdateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		Permissions: []string{"workspace:read", "workspace:manage"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(updated.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %v", updated.Permissions)
	}
	if updated.Name != "deployer" {
		t.Errorf("expected the name to stay unchanged, got %q", updated.Name)
	}

	// The same id, refetched, must reflect the new permission set - a
	// real in-place update, not a copy the caller just happens to hold.
	refetched, err := repo.GetByID(context.Background(), testOrgID, role.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(refetched.Permissions) != 2 {
		t.Errorf("expected the stored role to reflect the update, got %v", refetched.Permissions)
	}
}

func TestUpdateRoleService_RejectsUnknownPermission(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	repo := newFakeRoleRepo()
	role, _ := domain.NewRole(testOrgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	repo.put(role)
	svc := application.NewUpdateRoleService(repo, membership, perm)

	_, err := svc.Execute(context.Background(), application.UpdateRoleInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", RoleID: role.ID,
		Permissions: []string{"not:a:real:permission"},
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unknown permission, got: %v", err)
	}
}
