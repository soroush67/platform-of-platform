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

func TestBlockMemberService_RequiresOrganizationManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	membershipRepo.add(org.ID, "target-user")
	svc := application.NewBlockMemberService(membershipRepo, newFakePermChecker())

	err := svc.Execute(context.Background(), application.BlockMemberInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", TargetUserID: "target-user",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
	if membershipRepo.isBlocked(org.ID, "target-user") {
		t.Error("expected the target to remain unblocked after a forbidden attempt")
	}
}

func TestBlockMemberService_BlocksTarget(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "target-user")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	svc := application.NewBlockMemberService(membershipRepo, permChecker)

	if err := svc.Execute(context.Background(), application.BlockMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !membershipRepo.isBlocked(org.ID, "target-user") {
		t.Error("expected the target to be blocked")
	}

	// A blocked member fails IsMember identically to a non-member - the
	// whole mechanism BlockMemberService's own doc comment describes.
	isMember, _ := membershipRepo.IsMember(context.Background(), org.ID, "target-user")
	if isMember {
		t.Error("expected IsMember to return false for a blocked member")
	}
	// But the membership row itself still exists - a blocked member is
	// not the same as a removed one.
	if !membershipRepo.exists(org.ID, "target-user") {
		t.Error("expected the membership row to still exist after blocking")
	}
}

func TestUnblockMemberService_UnblocksAnAlreadyBlockedTarget(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "target-user")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	blockSvc := application.NewBlockMemberService(membershipRepo, permChecker)
	if err := blockSvc.Execute(context.Background(), application.BlockMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user",
	}); err != nil {
		t.Fatalf("Block Execute: %v", err)
	}

	// Regression check for the exact bug the plan called out: a naive
	// target-validation using IsMember (blocked-aware) would make an
	// already-blocked target permanently unblockable, since IsMember
	// itself now returns false for them.
	unblockSvc := application.NewUnblockMemberService(membershipRepo, permChecker)
	if err := unblockSvc.Execute(context.Background(), application.UnblockMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user",
	}); err != nil {
		t.Fatalf("expected unblocking an already-blocked target to succeed, got: %v", err)
	}
	if membershipRepo.isBlocked(org.ID, "target-user") {
		t.Error("expected the target to be unblocked")
	}
}

func TestRemoveMemberService_RequiresOrganizationManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	membershipRepo.add(org.ID, "target-user")
	cleaner := newFakeRoleBindingCleaner()
	svc := application.NewRemoveMemberService(membershipRepo, newFakePermChecker(), cleaner)

	err := svc.Execute(context.Background(), application.RemoveMemberInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", TargetUserID: "target-user",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
	if !membershipRepo.exists(org.ID, "target-user") {
		t.Error("expected the membership to still exist after a forbidden attempt")
	}
}

func TestRemoveMemberService_RemovesMembershipAndCleansUpRoleBindings(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "target-user")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	cleaner := newFakeRoleBindingCleaner()
	svc := application.NewRemoveMemberService(membershipRepo, permChecker, cleaner)

	if err := svc.Execute(context.Background(), application.RemoveMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if membershipRepo.exists(org.ID, "target-user") {
		t.Error("expected the membership row to be gone")
	}
	if !cleaner.calledFor(org.ID, "user", "target-user") {
		t.Error("expected RoleBindingCleaner.DeleteForSubject to be called for the removed user")
	}
}

func TestRemoveMemberService_AlreadyBlockedTargetCanStillBeRemoved(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "target-user")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	if err := application.NewBlockMemberService(membershipRepo, permChecker).Execute(context.Background(), application.BlockMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user",
	}); err != nil {
		t.Fatalf("Block Execute: %v", err)
	}

	svc := application.NewRemoveMemberService(membershipRepo, permChecker, newFakeRoleBindingCleaner())
	if err := svc.Execute(context.Background(), application.RemoveMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TargetUserID: "target-user",
	}); err != nil {
		t.Fatalf("expected removing an already-blocked member to succeed, got: %v", err)
	}
	if membershipRepo.exists(org.ID, "target-user") {
		t.Error("expected the membership row to be gone")
	}
}
