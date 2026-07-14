package application

import (
	"context"
	"log/slog"
	"time"
)

// StaleRunReaperService implements the Runnable interface
// (docs/architecture/18-backend-structure.md §4), same shape as
// internal/platform/outbox.Relay - registered directly in main.go's
// errgroup.
//
// docs/architecture/07-module-execution.md §3 names this "the single
// most commonly-missed piece in a first-pass execution-engine design."
// TriggerRunService/RunDispatchService's own retry path only covers "no
// Worker was connected at dispatch time" (RunDispatchService reverts to
// `queued` and lets the Outbox Relay's redelivery try again). It does
// NOT cover a Worker that successfully received a JobAssignment and
// then died mid-Job (crashed, network partition, OOM-killed) - nothing
// else in this codebase would ever notice, and the Run would stay
// `applying` forever, holding its Workspace's lock forever. This is
// the dedicated mechanism for that second case: periodically find Runs
// that have been `applying` longer than staleAfter and error them out.
type StaleRunReaperService struct {
	runRepo    RunRepository
	locker     WorkspaceLocker
	staleAfter time.Duration
	interval   time.Duration
	logger     *slog.Logger
}

func NewStaleRunReaperService(runRepo RunRepository, locker WorkspaceLocker, staleAfter, interval time.Duration, logger *slog.Logger) *StaleRunReaperService {
	return &StaleRunReaperService{runRepo: runRepo, locker: locker, staleAfter: staleAfter, interval: interval, logger: logger}
}

func (s *StaleRunReaperService) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.reapOnce(ctx); err != nil {
				// A transient DB error shouldn't kill the whole Reaper
				// loop - same posture as outbox.Relay's own batch-level
				// error handling.
				s.logger.Error("stale run reaper sweep failed", "error", err)
			}
		}
	}
}

func (s *StaleRunReaperService) reapOnce(ctx context.Context) error {
	cutoff := time.Now().Add(-s.staleAfter)

	candidates, err := s.runRepo.FindStaleApplyingRuns(ctx, cutoff)
	if err != nil {
		return err
	}

	for _, c := range candidates {
		reaped, err := s.runRepo.MarkErroredIfStillApplying(ctx, c.OrganizationID, c.RunID)
		if err != nil {
			s.logger.Error("failed to reap stale run", "run_id", c.RunID, "error", err)
			continue
		}
		if !reaped {
			// It reached a real terminal status on its own between the
			// RunApplying event firing and this sweep running - not
			// actually stale, nothing to do.
			continue
		}

		if err := s.locker.Unlock(ctx, c.OrganizationID, c.WorkspaceID, c.RunID); err != nil {
			s.logger.Error("reaped run but failed to unlock workspace", "run_id", c.RunID, "workspace_id", c.WorkspaceID, "error", err)
			continue
		}

		s.logger.Info("reaped stale run", "run_id", c.RunID, "organization_id", c.OrganizationID, "workspace_id", c.WorkspaceID)
	}

	return nil
}
