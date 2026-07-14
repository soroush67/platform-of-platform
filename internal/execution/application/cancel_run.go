package application

import (
	"context"

	"platform-of-platform/internal/execution/domain"
)

// CancelRunService implements
// `POST .../runs/{run}/cancel`. Same workspace:apply permission as
// triggering - whoever can start a Run can stop it.
type CancelRunService struct {
	runRepo  RunRepository
	locker   WorkspaceLocker
	perm     PermissionChecker
	canceler WorkerCanceler
}

func NewCancelRunService(runRepo RunRepository, locker WorkspaceLocker, perm PermissionChecker, canceler WorkerCanceler) *CancelRunService {
	return &CancelRunService{runRepo: runRepo, locker: locker, perm: perm, canceler: canceler}
}

func (s *CancelRunService) Execute(ctx context.Context, organizationID, workspaceID, runID, requestingUserID string) (*domain.Run, error) {
	allowed, err := s.perm.HasPermission(ctx, organizationID, requestingUserID, permissionWorkspaceApply)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	run, err := s.runRepo.GetByID(ctx, organizationID, runID)
	if err != nil {
		return nil, err
	}
	if run.WorkspaceID != workspaceID {
		return nil, domain.ErrRunNotFound
	}

	if err := run.Cancel(); err != nil {
		return nil, err
	}

	if err := s.runRepo.Update(ctx, run, requestingUserID); err != nil {
		return nil, err
	}

	// Release the workspace lock this Run held - the domain-level
	// Cancel() above already rejected an already-terminal Run, so
	// reaching here means this Run really was the lock holder (or the
	// lock was never acquired for some other reason, in which case
	// Unlock's own locked_by_run_id = runID guard is a no-op, not an
	// error - see the workspace adapter's own comment on that guard).
	if err := s.locker.Unlock(ctx, organizationID, workspaceID, run.ID); err != nil {
		return nil, err
	}

	// Tell the Worker to actually stop the real subprocess
	// (docs/architecture/17-workers.md §6) - deliberately best-effort,
	// after the DB-level cancellation above, not a precondition for it:
	// the Run is already, authoritatively, `canceled` regardless of
	// whether a live Worker is still reachable to notify. Ignoring both
	// return values is a deliberate choice, not an oversight - there is
	// nothing left for this operation to do differently based on
	// whether a Worker was actually running this Run or not.
	_, _ = s.canceler.CancelJob(ctx, run.ID)

	return run, nil
}
