package application

import (
	"context"

	"platform-of-platform/internal/execution/domain"
)

// ListRunsService - gated by canAccessWorkspace (project_visibility.go)
// since this session's per-project visibility change - previously
// membership-only, any role (same as every other read in this
// codebase).
type ListRunsService struct {
	runRepo           RunRepository
	membership        MembershipChecker
	workspaceChecker  WorkspaceChecker
	permChecker       PermissionChecker
	visibilityChecker VisibilityChecker
}

func NewListRunsService(runRepo RunRepository, membership MembershipChecker, workspaceChecker WorkspaceChecker, permChecker PermissionChecker, visibilityChecker VisibilityChecker) *ListRunsService {
	return &ListRunsService{runRepo: runRepo, membership: membership, workspaceChecker: workspaceChecker, permChecker: permChecker, visibilityChecker: visibilityChecker}
}

func (s *ListRunsService) Execute(ctx context.Context, organizationID, projectID, workspaceID, requestingUserID string) ([]*domain.Run, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrWorkspaceNotFound
	}

	exists, err := s.workspaceChecker.WorkspaceExists(ctx, organizationID, projectID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrWorkspaceNotFound
	}

	canAccess, err := canAccessWorkspace(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, projectID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, domain.ErrForbidden
	}

	return s.runRepo.ListByWorkspace(ctx, organizationID, workspaceID)
}
