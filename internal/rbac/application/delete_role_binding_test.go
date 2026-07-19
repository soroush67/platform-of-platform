package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/rbac/application"
	"platform-of-platform/internal/rbac/domain"
)

func TestDeleteRoleBindingService_RequiresOrganizationManage(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	bindingRepo := newFakeRoleBindingRepo()
	binding := domain.NewRoleBinding(testOrgID, "role-1", domain.SubjectTypeUser, "user-1", domain.ScopeTypeOrganization, testOrgID, domain.EffectAllow)
	_ = bindingRepo.Create(context.Background(), binding)
	svc := application.NewDeleteRoleBindingService(bindingRepo, membership, newFakePermissionChecker())

	err := svc.Execute(context.Background(), application.DeleteRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", BindingID: binding.ID,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
	if bindingRepo.get(binding.ID) == nil {
		t.Error("expected the binding to still exist after a forbidden delete attempt")
	}
}

func TestDeleteRoleBindingService_RemovesTheBinding(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	bindingRepo := newFakeRoleBindingRepo()
	binding := domain.NewRoleBinding(testOrgID, "role-1", domain.SubjectTypeUser, "user-1", domain.ScopeTypeOrganization, testOrgID, domain.EffectAllow)
	_ = bindingRepo.Create(context.Background(), binding)
	svc := application.NewDeleteRoleBindingService(bindingRepo, membership, perm)

	if err := svc.Execute(context.Background(), application.DeleteRoleBindingInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", BindingID: binding.ID,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if bindingRepo.get(binding.ID) != nil {
		t.Error("expected the binding to be permanently removed")
	}
}
