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
