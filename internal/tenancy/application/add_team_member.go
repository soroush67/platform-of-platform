package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// AddTeamMemberInput implements
// `POST /orgs/{org}/teams/{team}/members` (docs/architecture/13-module-
// identity-rbac-tenancy.md §1): { user_id }.
type AddTeamMemberInput struct {
	OrganizationID   string
	RequestingUserID string
	TeamID           string
	NewMemberUserID  string
}

type AddTeamMemberService struct {
	teamRepo       TeamRepository
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewAddTeamMemberService(teamRepo TeamRepository, membershipRepo MembershipRepository, permChecker PermissionChecker) *AddTeamMemberService {
	return &AddTeamMemberService{teamRepo: teamRepo, membershipRepo: membershipRepo, permChecker: permChecker}
}

func (s *AddTeamMemberService) Execute(ctx context.Context, in AddTeamMemberInput) error {
	isMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	team, err := s.teamRepo.GetByID(ctx, in.OrganizationID, in.TeamID)
	if err != nil {
		return err
	}

	// The user being added to a Team must themselves already be an
	// Organization member - a Team is a grouping *within* an org's
	// membership, not a way to grant org access in the first place
	// (that's still AddMemberService's job).
	targetIsMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.NewMemberUserID)
	if err != nil {
		return err
	}
	if !targetIsMember {
		return &domain.ValidationError{Message: "user_id is not a member of this organization"}
	}

	membership := domain.NewTeamMembership(team.ID, in.OrganizationID, in.NewMemberUserID)
	return s.teamRepo.AddMember(ctx, membership)
}
