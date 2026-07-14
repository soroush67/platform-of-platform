package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

// ListRolesService implements `GET /orgs/{org}/roles` - "lists built-in
// + org-custom roles" (docs/architecture/13-module-identity-rbac-
// tenancy.md §3). Read-only, so just membership-gated, same as every
// other list endpoint in this codebase (organization:read is implicit
// in being a member at all, per every other List*Service's own
// comment on this).
type ListRolesService struct {
	repo       RoleRepository
	membership MembershipChecker
}

func NewListRolesService(repo RoleRepository, membership MembershipChecker) *ListRolesService {
	return &ListRolesService{repo: repo, membership: membership}
}

func (s *ListRolesService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Role, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListForOrganization(ctx, organizationID)
}
