package application

import (
	"context"
	"log/slog"
	"time"
)

// PurgeRepository is the narrow port PurgeReaperService needs -
// separate from OrganizationRepository since no other caller needs
// these two methods.
type PurgeRepository interface {
	FindOrganizationsPastPurgeWindow(ctx context.Context, archivedBefore time.Time) ([]string, error)
	Purge(ctx context.Context, organizationID string) error
}

// PurgeReaperService implements the Runnable interface
// (docs/architecture/18-backend-structure.md §4), same shape as
// execution.StaleRunReaperService - registered directly in main.go's
// errgroup. Closes the second half of docs/architecture/13-module-
// identity-rbac-tenancy.md §1's "DELETE /orgs/{org} sets status:
// archived... schedules a background purge job 30 days out" - Archive
// (ArchiveOrganizationService) does the first half synchronously, this
// does the second half asynchronously, on a real timer, the same
// "periodic sweep, not blocked on a request" shape as the Stale Run
// Reaper.
type PurgeReaperService struct {
	repo       PurgeRepository
	purgeAfter time.Duration
	interval   time.Duration
	logger     *slog.Logger
}

func NewPurgeReaperService(repo PurgeRepository, purgeAfter, interval time.Duration, logger *slog.Logger) *PurgeReaperService {
	return &PurgeReaperService{repo: repo, purgeAfter: purgeAfter, interval: interval, logger: logger}
}

func (s *PurgeReaperService) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.reapOnce(ctx); err != nil {
				// A transient DB error shouldn't kill the whole Reaper
				// loop - same posture as outbox.Relay/StaleRunReaperService's
				// own batch-level error handling.
				s.logger.Error("purge reaper sweep failed", "error", err)
			}
		}
	}
}

func (s *PurgeReaperService) reapOnce(ctx context.Context) error {
	cutoff := time.Now().Add(-s.purgeAfter)

	orgIDs, err := s.repo.FindOrganizationsPastPurgeWindow(ctx, cutoff)
	if err != nil {
		return err
	}

	for _, orgID := range orgIDs {
		if err := s.repo.Purge(ctx, orgID); err != nil {
			s.logger.Error("failed to purge organization", "organization_id", orgID, "error", err)
			continue
		}
		s.logger.Info("purged archived organization", "organization_id", orgID)
	}

	return nil
}
