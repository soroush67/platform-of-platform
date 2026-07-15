package idempotency

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Reaper implements the Runnable interface (docs/architecture/
// 18-backend-structure.md §4), same shape as tenancy.PurgeReaperService/
// execution.StaleRunReaperService - registered in main.go's errgroup.
// Closes the gap migrations/0011_idempotency_keys.up.sql's own comment
// named: "rows past the window aren't actively deleted... a real,
// deferred gap: this table grows by one row per distinct idempotency
// key ever used, unbounded." A periodic sweep, not blocked on any
// request, deleting exactly the rows Get already treats as expired
// (both derive their cutoff from the same Window constant - see
// store.go's own comment on why that matters).
//
// idempotency_keys has FORCE ROW LEVEL SECURITY (organization-scoped),
// but this cleanup is inherently cross-org - there's no single
// organization_id to set_config for a sweep that has to consider every
// org's expired keys in one pass. Unlike FindStaleApplyingRuns/
// FindOrganizationsPastPurgeWindow (which sidestep this by reading
// outbox_events, a table with no RLS at all), idempotency_keys itself
// genuinely has RLS, so there's no non-root way to see across every
// org's rows in a single DELETE. Reaper is therefore constructed with
// the root connection (same role migrations use, main.go's own
// rootPool), not the app pool platform_app/every other Store method
// uses - root's RLS bypass here is the same legitimate "real,
// cross-tenant admin operation" case it already exists for, applied to
// a new, honestly-documented case rather than silently reusing
// platform_app and getting a reaper that deletes zero rows forever
// (the exact class of bug Purge's own comment describes finding for
// real with a plain, RLS-unaware query).
type Reaper struct {
	rootPool *pgxpool.Pool
	interval time.Duration
	logger   *slog.Logger
}

func NewReaper(rootPool *pgxpool.Pool, interval time.Duration, logger *slog.Logger) *Reaper {
	return &Reaper{rootPool: rootPool, interval: interval, logger: logger}
}

func (r *Reaper) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.reapOnce(ctx); err != nil {
				// A transient DB error shouldn't kill the whole Reaper
				// loop - same posture as every other Reaper/Relay in
				// this codebase.
				r.logger.Error("idempotency key reaper sweep failed", "error", err)
			}
		}
	}
}

func (r *Reaper) reapOnce(ctx context.Context) error {
	cutoff := time.Now().Add(-Window)
	tag, err := r.rootPool.Exec(ctx, `DELETE FROM idempotency_keys WHERE created_at <= $1`, cutoff)
	if err != nil {
		return err
	}
	if n := tag.RowsAffected(); n > 0 {
		r.logger.Info("purged expired idempotency keys", "count", n)
	}
	return nil
}
