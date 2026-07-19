package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

// ListWorkspacesService - gated by canAccessProject (project_visibility.go)
// since this session's per-project visibility change: previously
// membership-only, meaning every org member could list every Project's
// Workspaces regardless of role.
type ListWorkspacesService struct {
	repo              WorkspaceRepository
	membership        MembershipChecker
	projectChecker    ProjectChecker
	permChecker       PermissionChecker
	visibilityChecker VisibilityChecker
}

func NewListWorkspacesService(repo WorkspaceRepository, membership MembershipChecker, projectChecker ProjectChecker, permChecker PermissionChecker, visibilityChecker VisibilityChecker) *ListWorkspacesService {
	return &ListWorkspacesService{repo: repo, membership: membership, projectChecker: projectChecker, permChecker: permChecker, visibilityChecker: visibilityChecker}
}

func (s *ListWorkspacesService) Execute(ctx context.Context, organizationID, projectID, requestingUserID string) ([]*domain.Workspace, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrProjectNotFound
	}

	exists, err := s.projectChecker.ProjectExists(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	canAccess, err := canAccessProject(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, projectID)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListByProject(ctx, organizationID, projectID)
}
