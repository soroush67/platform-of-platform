package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// GetProjectService implements `GET /api/v1/orgs/{org}/projects/{project}`.
// Read access is membership-gated only (any org member, any role, per
// the built-in roles' shared organization:read permission) - unlike
// creation, reading a project isn't an org-structural action, so it
// doesn't need organization:manage.
type GetProjectService struct {
	repo           ProjectRepository
	membershipRepo MembershipRepository
}

func NewGetProjectService(repo ProjectRepository, membershipRepo MembershipRepository) *GetProjectService {
	return &GetProjectService{repo: repo, membershipRepo: membershipRepo}
}

func (s *GetProjectService) Execute(ctx context.Context, organizationID, projectID, requestingUserID string) (*domain.Project, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		// Same choice as ListProjectsService: a non-member gets "org not
		// found," not "project not found" - ErrProjectNotFound is
		// reserved for "you're a real member, but this project id isn't
		// in this org," which repo.GetByID below returns.
		return nil, domain.ErrOrganizationNotFound
	}

	return s.repo.GetByID(ctx, organizationID, projectID)
}
