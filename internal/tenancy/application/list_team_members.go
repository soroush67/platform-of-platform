package application

import (
	"context"
	"time"

	"platform-of-platform/internal/tenancy/domain"
)

// TeamMemberSummary mirrors MemberSummary's own reasoning (list_members.go)
// - a read-model composed from Tenancy's own team_memberships row and
// Identity's user record, not a domain aggregate. No RoleName field -
// unlike organization membership, Team membership itself carries no
// per-team role (a Team is purely an RBAC *subject*, matching the RBAC
// per-menu access-control redesign - "role" only exists as a RoleBinding
// bound to a Team, not on the membership row itself).
type TeamMemberSummary struct {
	UserID   string
	Username string
	Email    string
	JoinedAt time.Time
}

// ListTeamMembersService implements `GET /orgs/{org}/teams/{team}/members` -
// closes the real gap TeamsPage.tsx hit: there was no way to see who's
// actually in a Team, so "Add member" had no visible confirmation and
// "Remove member" was a blind guess from the org-wide roster. Same
// membership-only gate as ListMembersService (any org member can see a
// Team's roster) - GetByID's own org-ownership check is what a non-
// member/wrong-org team id fails against.
type ListTeamMembersService struct {
	teamRepo       TeamRepository
	membershipRepo MembershipRepository
	userReader     UserReader
}

func NewListTeamMembersService(teamRepo TeamRepository, membershipRepo MembershipRepository, userReader UserReader) *ListTeamMembersService {
	return &ListTeamMembersService{teamRepo: teamRepo, membershipRepo: membershipRepo, userReader: userReader}
}

func (s *ListTeamMembersService) Execute(ctx context.Context, organizationID, teamID, requestingUserID string) ([]TeamMemberSummary, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	if _, err := s.teamRepo.GetByID(ctx, organizationID, teamID); err != nil {
		return nil, err
	}

	memberships, err := s.teamRepo.ListMembers(ctx, organizationID, teamID)
	if err != nil {
		return nil, err
	}

	summaries := make([]TeamMemberSummary, 0, len(memberships))
	for _, m := range memberships {
		username, email, _, err := s.userReader.GetUser(ctx, m.UserID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, TeamMemberSummary{
			UserID:   m.UserID,
			Username: username,
			Email:    email,
			JoinedAt: m.JoinedAt,
		})
	}

	return summaries, nil
}
