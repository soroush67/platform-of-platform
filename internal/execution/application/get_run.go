package application

import (
	"context"

	"platform-of-platform/internal/execution/domain"
)

type GetRunService struct {
	runRepo          RunRepository
	membership       MembershipChecker
	workspaceChecker WorkspaceChecker
}

func NewGetRunService(runRepo RunRepository, membership MembershipChecker, workspaceChecker WorkspaceChecker) *GetRunService {
	return &GetRunService{runRepo: runRepo, membership: membership, workspaceChecker: workspaceChecker}
}

func (s *GetRunService) Execute(ctx context.Context, organizationID, projectID, workspaceID, runID, requestingUserID string) (*domain.Run, error) {
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

	run, err := s.runRepo.GetByID(ctx, organizationID, runID)
	if err != nil {
		return nil, err
	}
	if run.WorkspaceID != workspaceID {
		return nil, domain.ErrRunNotFound
	}

	return run, nil
}
