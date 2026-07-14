package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// ListProjectsService implements `GET /api/v1/orgs/{org}/projects` -
// same membership-only gate as GetProjectService.
type ListProjectsService struct {
	repo           ProjectRepository
	membershipRepo MembershipRepository
}

func NewListProjectsService(repo ProjectRepository, membershipRepo MembershipRepository) *ListProjectsService {
	return &ListProjectsService{repo: repo, membershipRepo: membershipRepo}
}

func (s *ListProjectsService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Project, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	return s.repo.ListByOrganization(ctx, organizationID)
}
