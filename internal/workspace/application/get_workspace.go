package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

// GetWorkspaceService - gated by canAccessWorkspace (project_visibility.go),
// same reasoning as ListWorkspacesService above.
type GetWorkspaceService struct {
	repo              WorkspaceRepository
	membership        MembershipChecker
	projectChecker    ProjectChecker
	permChecker       PermissionChecker
	visibilityChecker VisibilityChecker
}

func NewGetWorkspaceService(repo WorkspaceRepository, membership MembershipChecker, projectChecker ProjectChecker, permChecker PermissionChecker, visibilityChecker VisibilityChecker) *GetWorkspaceService {
	return &GetWorkspaceService{repo: repo, membership: membership, projectChecker: projectChecker, permChecker: permChecker, visibilityChecker: visibilityChecker}
}

func (s *GetWorkspaceService) Execute(ctx context.Context, organizationID, projectID, workspaceID, requestingUserID string) (*domain.Workspace, error) {
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

	canAccess, err := canAccessWorkspace(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, projectID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, domain.ErrForbidden
	}

	ws, err := s.repo.GetByID(ctx, organizationID, workspaceID)
	if err != nil {
		return nil, err
	}
	if ws.ProjectID != projectID {
		return nil, domain.ErrWorkspaceNotFound
	}

	return ws, nil
}
