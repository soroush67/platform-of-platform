package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// RemoveTeamMemberInput implements
// `DELETE /orgs/{org}/teams/{team}/members/{user_id}`.
type RemoveTeamMemberInput struct {
	OrganizationID   string
	RequestingUserID string
	TeamID           string
	MemberUserID     string
}

type RemoveTeamMemberService struct {
	teamRepo       TeamRepository
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewRemoveTeamMemberService(teamRepo TeamRepository, membershipRepo MembershipRepository, permChecker PermissionChecker) *RemoveTeamMemberService {
	return &RemoveTeamMemberService{teamRepo: teamRepo, membershipRepo: membershipRepo, permChecker: permChecker}
}

func (s *RemoveTeamMemberService) Execute(ctx context.Context, in RemoveTeamMemberInput) error {
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

	return s.teamRepo.RemoveMember(ctx, in.OrganizationID, team.ID, in.MemberUserID)
}
