package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// DeleteProjectRepository is the narrow extra port DeleteProjectService
// needs beyond ProjectRepository's own Create/GetByID/ListByOrganization -
// Purge is a genuinely irreversible hard delete (no archive/purge-window
// two-stage flow the way Organization has - project:manage is already
// Owner/Admin-only, the same "structural change" gate CreateProjectService
// itself uses, so a second, stricter permission wasn't warranted).
type DeleteProjectRepository interface {
	Purge(ctx context.Context, organizationID, projectID string) error
}

// DeleteProjectInput implements `DELETE /api/v1/orgs/{org}/projects/{project}`.
// Gated by project:delete - a stricter, Owner-only permission than
// project:manage (which CreateProjectService checks), matching
// organization:delete's own precedent of narrowing destructive delete
// below what *:manage otherwise grants. Org-wide, not canAccessProject's
// per-project visibility gate (that gate is read-side only, see
// project_visibility.go's own doc comment).
type DeleteProjectInput struct {
	OrganizationID   string
	ProjectID        string
	RequestingUserID string
}

type DeleteProjectService struct {
	repo           ProjectRepository
	deleteRepo     DeleteProjectRepository
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewDeleteProjectService(repo ProjectRepository, deleteRepo DeleteProjectRepository, membershipRepo MembershipRepository, permChecker PermissionChecker) *DeleteProjectService {
	return &DeleteProjectService{repo: repo, deleteRepo: deleteRepo, membershipRepo: membershipRepo, permChecker: permChecker}
}

func (s *DeleteProjectService) Execute(ctx context.Context, in DeleteProjectInput) error {
	isMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionProjectDelete)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	// Confirms the project genuinely exists in this org before calling
	// Purge - same reasoning DeleteOrganizationService's own Execute
	// documents for GetByID before Purge.
	if _, err := s.repo.GetByID(ctx, in.OrganizationID, in.ProjectID); err != nil {
		return err
	}

	return s.deleteRepo.Purge(ctx, in.OrganizationID, in.ProjectID)
}
