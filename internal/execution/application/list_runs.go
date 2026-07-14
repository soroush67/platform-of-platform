package application

import (
	"context"

	"platform-of-platform/internal/execution/domain"
)

// ListRunsService - membership-gated only, any role (same as every
// other read in this codebase).
type ListRunsService struct {
	runRepo          RunRepository
	membership       MembershipChecker
	workspaceChecker WorkspaceChecker
}

func NewListRunsService(runRepo RunRepository, membership MembershipChecker, workspaceChecker WorkspaceChecker) *ListRunsService {
	return &ListRunsService{runRepo: runRepo, membership: membership, workspaceChecker: workspaceChecker}
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

	return s.runRepo.ListByWorkspace(ctx, organizationID, workspaceID)
}
