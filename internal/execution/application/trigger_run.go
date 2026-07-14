package application

import (
	"context"

	"platform-of-platform/internal/execution/domain"
)

const permissionWorkspaceApply = "workspace:apply"

// TriggerRunInput implements
// `POST /api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}/runs`.
type TriggerRunInput struct {
	OrganizationID   string
	ProjectID        string
	WorkspaceID      string
	RequestingUserID string
}

type TriggerRunService struct {
	runRepo          RunRepository
	locker           WorkspaceLocker
	workspaceChecker WorkspaceChecker
	permChecker      PermissionChecker
}

func NewTriggerRunService(runRepo RunRepository, locker WorkspaceLocker, workspaceChecker WorkspaceChecker, permChecker PermissionChecker) *TriggerRunService {
	return &TriggerRunService{runRepo: runRepo, locker: locker, workspaceChecker: workspaceChecker, permChecker: permChecker}
}

func (s *TriggerRunService) Execute(ctx context.Context, in TriggerRunInput) (*domain.Run, error) {
	exists, err := s.workspaceChecker.WorkspaceExists(ctx, in.OrganizationID, in.ProjectID, in.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrWorkspaceNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionWorkspaceApply)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	run, err := domain.NewRun(in.OrganizationID, in.WorkspaceID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}

	// Lock *before* persisting the Run - the common, expected case
	// (workspace already locked by another in-flight Run) then never
	// creates an orphan row at all. The rarer case - lock succeeds but
	// the subsequent Create fails for some other reason - is
	// compensated with a best-effort Unlock below; this isn't a full
	// saga/outbox pattern (Stage 6 territory, not built in this
	// codebase yet), a known, flagged simplification rather than a
	// silently-accepted risk.
	locked, err := s.locker.TryLock(ctx, in.OrganizationID, in.WorkspaceID, run.ID)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, domain.ErrWorkspaceLocked
	}

	if err := s.runRepo.Create(ctx, run); err != nil {
		_ = s.locker.Unlock(ctx, in.OrganizationID, in.WorkspaceID, run.ID)
		return nil, err
	}

	return run, nil
}
