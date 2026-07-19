package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// UpdateTeamInput implements `PUT /orgs/{org}/teams/{team}` - a rename,
// nothing else (Team has no other mutable field). Same organization:manage
// gate as CreateTeamService/AddTeamMemberService.
type UpdateTeamInput struct {
	OrganizationID   string
	RequestingUserID string
	TeamID           string
	Name             string
}

type UpdateTeamService struct {
	repo        TeamRepository
	membership  MembershipRepository
	permChecker PermissionChecker
}

func NewUpdateTeamService(repo TeamRepository, membership MembershipRepository, permChecker PermissionChecker) *UpdateTeamService {
	return &UpdateTeamService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *UpdateTeamService) Execute(ctx context.Context, in UpdateTeamInput) (*domain.Team, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	if in.Name == "" {
		return nil, &domain.ValidationError{Message: "name is required"}
	}

	team, err := s.repo.GetByID(ctx, in.OrganizationID, in.TeamID)
	if err != nil {
		return nil, err
	}

	team.Name = in.Name
	if err := s.repo.Update(ctx, team); err != nil {
		return nil, err
	}

	return team, nil
}
