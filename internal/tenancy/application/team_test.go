package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func TestCreateTeamService_RequiresOrganizationManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	teamRepo := newFakeTeamRepo()
	svc := application.NewCreateTeamService(teamRepo, membershipRepo, newFakePermChecker())

	_, err := svc.Execute(context.Background(), application.CreateTeamInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", Name: "platform-team",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestAddTeamMemberService_TargetMustAlreadyBeOrgMember(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platform-team")
	_ = teamRepo.Create(context.Background(), team)
	svc := application.NewAddTeamMemberService(teamRepo, membershipRepo, permChecker)

	// Not yet an org member - Team membership can't be the *first* grant
	// of org access (that's still AddMemberService's job).
	err := svc.Execute(context.Background(), application.AddTeamMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TeamID: team.ID, NewMemberUserID: "outsider",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for a non-org-member target, got: %v", err)
	}

	membershipRepo.add(org.ID, "outsider")
	if err := svc.Execute(context.Background(), application.AddTeamMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TeamID: team.ID, NewMemberUserID: "outsider",
	}); err != nil {
		t.Fatalf("expected success once the target is a real org member, got: %v", err)
	}
	if !teamRepo.isMember(team.ID, "outsider") {
		t.Error("expected the user to now be a team member")
	}
}

func TestRemoveTeamMemberService_RemovesRealMembership(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "team-member")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platform-team")
	_ = teamRepo.Create(context.Background(), team)
	_ = teamRepo.AddMember(context.Background(), domain.NewTeamMembership(team.ID, org.ID, "team-member"))

	svc := application.NewRemoveTeamMemberService(teamRepo, membershipRepo, permChecker)
	if err := svc.Execute(context.Background(), application.RemoveTeamMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TeamID: team.ID, MemberUserID: "team-member",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if teamRepo.isMember(team.ID, "team-member") {
		t.Error("expected the membership to be removed")
	}
}

func TestRemoveTeamMemberService_UnknownTeamNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	svc := application.NewRemoveTeamMemberService(newFakeTeamRepo(), membershipRepo, permChecker)

	err := svc.Execute(context.Background(), application.RemoveTeamMemberInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TeamID: "nonexistent-team", MemberUserID: "x",
	})
	if !errors.Is(err, domain.ErrTeamNotFound) {
		t.Fatalf("expected ErrTeamNotFound, got: %v", err)
	}
}

func TestListTeamMembersService_ReturnsRosterWithUserInfo(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "requester")
	membershipRepo.add(org.ID, "member-a")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platform-team")
	_ = teamRepo.Create(context.Background(), team)
	_ = teamRepo.AddMember(context.Background(), domain.NewTeamMembership(team.ID, org.ID, "member-a"))

	userReader := newFakeUserReader()
	userReader.set("member-a", "alice", "alice@example.com")

	svc := application.NewListTeamMembersService(teamRepo, membershipRepo, userReader)
	got, err := svc.Execute(context.Background(), org.ID, team.ID, "requester")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].UserID != "member-a" || got[0].Username != "alice" {
		t.Errorf("expected exactly member-a resolved to alice, got %+v", got)
	}
}

func TestListTeamMembersService_UnknownTeamNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "requester")
	svc := application.NewListTeamMembersService(newFakeTeamRepo(), membershipRepo, newFakeUserReader())

	_, err := svc.Execute(context.Background(), org.ID, "nonexistent-team", "requester")
	if !errors.Is(err, domain.ErrTeamNotFound) {
		t.Fatalf("expected ErrTeamNotFound, got: %v", err)
	}
}

func TestUpdateTeamService_RequiresOrganizationManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platfrom-mamanger")
	_ = teamRepo.Create(context.Background(), team)
	svc := application.NewUpdateTeamService(teamRepo, membershipRepo, newFakePermChecker())

	_, err := svc.Execute(context.Background(), application.UpdateTeamInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", TeamID: team.ID, Name: "platform-manager",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestUpdateTeamService_RenamesInPlace(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platfrom-mamanger")
	_ = teamRepo.Create(context.Background(), team)
	svc := application.NewUpdateTeamService(teamRepo, membershipRepo, permChecker)

	updated, err := svc.Execute(context.Background(), application.UpdateTeamInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TeamID: team.ID, Name: "platform-manager",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if updated.Name != "platform-manager" {
		t.Errorf("expected the renamed name, got %q", updated.Name)
	}

	refetched, err := teamRepo.GetByID(context.Background(), org.ID, team.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if refetched.Name != "platform-manager" {
		t.Errorf("expected the stored team to reflect the rename, got %q", refetched.Name)
	}
}

func TestDeleteTeamService_RequiresOrganizationManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platform-team")
	_ = teamRepo.Create(context.Background(), team)
	cleaner := newFakeRoleBindingCleaner()
	svc := application.NewDeleteTeamService(teamRepo, membershipRepo, newFakePermChecker(), cleaner)

	err := svc.Execute(context.Background(), application.DeleteTeamInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", TeamID: team.ID,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
	if _, err := teamRepo.GetByID(context.Background(), org.ID, team.ID); err != nil {
		t.Error("expected the team to still exist after a forbidden delete attempt")
	}
}

func TestDeleteTeamService_DeletesTeamAndCleansUpRoleBindings(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin-1")
	membershipRepo.add(org.ID, "member-a")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin-1", "organization:manage")
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam(org.ID, "platform-team")
	_ = teamRepo.Create(context.Background(), team)
	_ = teamRepo.AddMember(context.Background(), domain.NewTeamMembership(team.ID, org.ID, "member-a"))
	cleaner := newFakeRoleBindingCleaner()
	svc := application.NewDeleteTeamService(teamRepo, membershipRepo, permChecker, cleaner)

	if err := svc.Execute(context.Background(), application.DeleteTeamInput{
		OrganizationID: org.ID, RequestingUserID: "admin-1", TeamID: team.ID,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := teamRepo.GetByID(context.Background(), org.ID, team.ID); !errors.Is(err, domain.ErrTeamNotFound) {
		t.Errorf("expected the team to be gone, got: %v", err)
	}
	members, err := teamRepo.ListMembers(context.Background(), org.ID, team.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected the team's own memberships to be gone too, got %+v", members)
	}
	if !cleaner.calledFor(org.ID, "team", team.ID) {
		t.Error("expected RoleBindingCleaner.DeleteForSubject to be called for the deleted team")
	}
}
