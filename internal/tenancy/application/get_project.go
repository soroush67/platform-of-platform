package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// GetProjectService implements `GET /api/v1/orgs/{org}/projects/{project}`.
// Gated by project:read (see the RBAC per-menu access-control redesign -
// previously membership-only).
type GetProjectService struct {
	repo           ProjectRepository
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewGetProjectService(repo ProjectRepository, membershipRepo MembershipRepository, permChecker PermissionChecker) *GetProjectService {
	return &GetProjectService{repo: repo, membershipRepo: membershipRepo, permChecker: permChecker}
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
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionProjectRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	return s.repo.GetByID(ctx, organizationID, projectID)
}
