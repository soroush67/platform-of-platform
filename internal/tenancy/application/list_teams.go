package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// ListTeamsService implements `GET /api/v1/orgs/{org}/teams` - same
// membership-only gate as ListMembersService (any org member can see
// the Team/"Group" roster, no organization:manage requirement - that's
// CreateTeamService's own, stricter gate). Closes the same
// missing-list-endpoint gap ListMembersService already closed once
// before: Teams are the "Group" concept the RBAC per-menu
// access-control redesign binds Roles to, so they need to be real,
// browsable data before RoleBindingsPage can offer a Team picker.
type ListTeamsService struct {
	repo           TeamRepository
	membershipRepo MembershipRepository
}

func NewListTeamsService(repo TeamRepository, membershipRepo MembershipRepository) *ListTeamsService {
	return &ListTeamsService{repo: repo, membershipRepo: membershipRepo}
}

func (s *ListTeamsService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Team, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	return s.repo.ListByOrganization(ctx, organizationID)
}
