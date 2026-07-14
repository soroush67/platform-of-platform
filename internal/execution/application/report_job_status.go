package application

import (
	"context"
	"errors"

	"platform-of-platform/internal/execution/domain"
)

// WorkerReportService.HandleReport is called from the gRPC adapter's
// Server.ReportJobStatus - the real path a connected Worker's
// ReportJobStatus RPC calls trigger. Not an outbox.Handler like
// RunDispatchService/RecordEntryService: this arrives over the live
// gRPC connection itself, not through the Outbox (a Worker's status
// report isn't a domain event another context subscribes to, it's a
// direct RPC response to work the Control Plane itself dispatched).
type WorkerReportService struct {
	runRepo RunRepository
	locker  WorkspaceLocker
}

func NewWorkerReportService(runRepo RunRepository, locker WorkspaceLocker) *WorkerReportService {
	return &WorkerReportService{runRepo: runRepo, locker: locker}
}

func (s *WorkerReportService) HandleReport(ctx context.Context, organizationID, runID, workspaceID, reportedStatus, errorMessage string) error {
	run, err := s.runRepo.GetByID(ctx, organizationID, runID)
	if err != nil {
		return err
	}

	switch reportedStatus {
	case "applied":
		if err := run.MarkApplied(); err != nil {
			if errors.Is(err, domain.ErrInvalidTransition) {
				// Already terminal - a duplicate/late report arriving
				// after this Run already finished. Treat as a benign
				// no-op rather than making the Worker retry forever.
				return nil
			}
			return err
		}
	case "failed", "errored":
		if err := run.MarkFailed(); err != nil {
			if errors.Is(err, domain.ErrInvalidTransition) {
				return nil
			}
			return err
		}
		// ApplyOutputRef is documented (Stage 5 §6) as an object storage
		// pointer - this codebase has no object storage wired up yet (no
		// State context, no MinIO), so the raw error text is stored
		// inline here instead, a small and honestly-flagged
		// simplification rather than a fabricated storage reference.
		if errorMessage != "" {
			run.ApplyOutputRef = &errorMessage
		}
	default:
		return &domain.ValidationError{Message: "status must be one of applied, failed, errored"}
	}

	if err := s.runRepo.Update(ctx, run, "system"); err != nil {
		return err
	}

	return s.locker.Unlock(ctx, organizationID, workspaceID, run.ID)
}
