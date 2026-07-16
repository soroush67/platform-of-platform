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

func TestListMembersService_NonMemberGetsOrganizationNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	svc := application.NewListMembersService(membershipRepo, newFakeUserReader(), newFakeRoleReader())

	_, err := svc.Execute(context.Background(), org.ID, "not-a-member")
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for a non-member requester, got: %v", err)
	}
}

func TestListMembersService_ReturnsRosterWithResolvedUserAndRole(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "member-2")

	userReader := newFakeUserReader()
	userReader.set("admin-1", "alice", "alice@example.com")
	userReader.set("member-2", "bob", "bob@example.com")
	roleReader := newFakeRoleReader()
	roleReader.set(org.ID, "admin-1", "owner")
	roleReader.set(org.ID, "member-2", "write")

	svc := application.NewListMembersService(membershipRepo, userReader, roleReader)
	got, err := svc.Execute(context.Background(), org.ID, "admin-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 roster rows, got %d: %+v", len(got), got)
	}
	byUser := map[string]application.MemberSummary{}
	for _, m := range got {
		byUser[m.UserID] = m
	}
	if byUser["admin-1"].Username != "alice" || byUser["admin-1"].RoleName != "owner" {
		t.Errorf("expected admin-1 to resolve to alice/owner, got %+v", byUser["admin-1"])
	}
	if byUser["member-2"].Email != "bob@example.com" || byUser["member-2"].RoleName != "write" {
		t.Errorf("expected member-2 to resolve to bob@example.com/write, got %+v", byUser["member-2"])
	}
}

func TestListMembersService_MemberWithNoRoleBindingShowsEmptyRole(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")

	userReader := newFakeUserReader()
	userReader.set("admin-1", "alice", "alice@example.com")
	// Deliberately no role set on roleReader - a member added outside
	// AddMemberService's own AssignRole call.
	svc := application.NewListMembersService(membershipRepo, userReader, newFakeRoleReader())

	got, err := svc.Execute(context.Background(), org.ID, "admin-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].RoleName != "" {
		t.Errorf("expected exactly one roster row with an empty role name, not an error, got %+v", got)
	}
}
