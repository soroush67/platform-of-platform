package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// ListMyOrganizationsService implements GET /api/v1/orgs - no membership
// gate needed (unlike ListProjectsService), the query itself
// (RootMembershipRepository.ListOrganizationsForUser) is already
// self-scoped to the caller's own user id.
type ListMyOrganizationsService struct {
	repo RootMembershipRepository
}

func NewListMyOrganizationsService(repo RootMembershipRepository) *ListMyOrganizationsService {
	return &ListMyOrganizationsService{repo: repo}
}

func (s *ListMyOrganizationsService) Execute(ctx context.Context, userID string) ([]*domain.Organization, error) {
	return s.repo.ListOrganizationsForUser(ctx, userID)
}
