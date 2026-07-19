package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// GetProjectService implements `GET /api/v1/orgs/{org}/projects/{project}`.
// Gated by canAccessProject (project_visibility.go) - an Owner/Admin
// bypasses it, everyone else needs a real project-scope grant (direct or
// via a Team) - previously a blanket org-wide project:read check, which
// every member already passed (see VisibilityChecker's own doc comment,
// ports.go, for why that was never actually restrictive).
type GetProjectService struct {
	repo              ProjectRepository
	membershipRepo    MembershipRepository
	permChecker       PermissionChecker
	visibilityChecker VisibilityChecker
}

func NewGetProjectService(repo ProjectRepository, membershipRepo MembershipRepository, permChecker PermissionChecker, visibilityChecker VisibilityChecker) *GetProjectService {
	return &GetProjectService{repo: repo, membershipRepo: membershipRepo, permChecker: permChecker, visibilityChecker: visibilityChecker}
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
	allowed, err := canAccessProject(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, projectID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	return s.repo.GetByID(ctx, organizationID, projectID)
}
