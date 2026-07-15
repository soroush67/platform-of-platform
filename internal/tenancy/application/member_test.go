package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func TestAddMemberService_GrantsDefaultReadRole(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	roleAssigner := newFakeRoleAssigner()
	svc := application.NewAddMemberService(membershipRepo, permChecker, roleAssigner)

	err := svc.Execute(context.Background(), application.AddMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", NewMemberUserID: "new-user",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	isMember, _ := membershipRepo.IsMember(context.Background(), org.ID, "new-user")
	if !isMember {
		t.Error("expected the new user to become a member")
	}
	if got := roleAssigner.roleOf(org.ID, "new-user"); got != "read" {
		t.Errorf("expected default role 'read', got %q", got)
	}
}

func TestAddMemberService_RequiresOrganizationManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "read-only-user")
	svc := application.NewAddMemberService(membershipRepo, newFakePermChecker(), newFakeRoleAssigner())

	err := svc.Execute(context.Background(), application.AddMemberInput{
		OrganizationID: org.ID, RequestingUserID: "read-only-user", NewMemberUserID: "new-user",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestChangeMemberRoleService_RejectsUnknownRoleName(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	svc := application.NewChangeMemberRoleService(membershipRepo, newFakeRoleAssigner(), newFakePermChecker())

	err := svc.Execute(context.Background(), application.ChangeMemberRoleInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "member-1", RoleName: "superadmin",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unknown role name, got: %v", err)
	}
}

func TestChangeMemberRoleService_TargetMustBeAnExistingMember(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	svc := application.NewChangeMemberRoleService(membershipRepo, newFakeRoleAssigner(), permChecker)

	err := svc.Execute(context.Background(), application.ChangeMemberRoleInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "never-added", RoleName: "write",
	})
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for a non-member target, got: %v", err)
	}
}

func TestChangeMemberRoleService_ReplacesNotAdds(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "target-user")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	roleChanger := newFakeRoleAssigner()
	svc := application.NewChangeMemberRoleService(membershipRepo, roleChanger, permChecker)

	if err := svc.Execute(context.Background(), application.ChangeMemberRoleInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user", RoleName: "write",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := roleChanger.roleOf(org.ID, "target-user"); got != "write" {
		t.Errorf("expected role 'write', got %q", got)
	}
}
