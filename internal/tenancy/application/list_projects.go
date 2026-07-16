package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// ListProjectsService implements `GET /api/v1/orgs/{org}/projects` -
// same gating as GetProjectService.
type ListProjectsService struct {
	repo           ProjectRepository
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewListProjectsService(repo ProjectRepository, membershipRepo MembershipRepository, permChecker PermissionChecker) *ListProjectsService {
	return &ListProjectsService{repo: repo, membershipRepo: membershipRepo, permChecker: permChecker}
}

func (s *ListProjectsService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Project, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionProjectRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListByOrganization(ctx, organizationID)
}
