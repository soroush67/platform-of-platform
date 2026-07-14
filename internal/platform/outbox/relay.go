package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler processes one already-committed event - typically translating
// it into a write in the subscriber's own table (Audit's
// RecordFromEvent is the first real one). Returning an error leaves the
// event unpublished, to be retried on the next batch.
type Handler func(ctx context.Context, event Event) error

// Relay implements the Runnable interface
// (docs/architecture/18-backend-structure.md §4: "type Runnable
// interface { Run(ctx context.Context) error }") - polls outbox_events
// for unpublished rows and dispatches them to Handler, at-least-once
// (docs/architecture/20-tests.md §2's own framing: "proving at-least-
// once, not exactly-once, which every consumer's idempotency depends on
// being true"). This walking skeleton's Handler (Audit's
// RecordFromEvent) is a plain INSERT with a fresh id each call, so a
// redelivery after a crash between a successful Handler call and the
// UPDATE that marks it published *would* duplicate the audit entry - a
// real, known gap (Stage 6 §5 names idempotent consumers as the
// expected fix, not built here) flagged rather than silently accepted
// as correct.
type Relay struct {
	pool     *pgxpool.Pool
	handler  Handler
	interval time.Duration
	logger   *slog.Logger
}

func NewRelay(pool *pgxpool.Pool, handler Handler, interval time.Duration, logger *slog.Logger) *Relay {
	return &Relay{pool: pool, handler: handler, interval: interval, logger: logger}
}

func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.relayBatch(ctx); err != nil {
				// A transient DB error shouldn't kill the whole relay
				// loop - log and let the next tick retry, same posture
				// as any other polling Runnable in this codebase would
				// need (there's only one so far, but this is the
				// pattern the doc names for all of them).
				r.logger.Error("outbox relay batch failed", "error", err)
			}
		}
	}
}

func (r *Relay) relayBatch(ctx context.Context) error {
	rows, err := r.pool.Query(ctx,
		`SELECT id, organization_id, event_type, payload, occurred_at
		 FROM outbox_events WHERE published_at IS NULL ORDER BY occurred_at LIMIT 100`,
	)
	if err != nil {
		return err
	}

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.EventType, &e.Payload, &e.OccurredAt); err != nil {
			rows.Close()
			return err
		}
		events = append(events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, e := range events {
		if err := r.handler(ctx, e); err != nil {
			r.logger.Error("outbox handler failed, will retry next batch", "event_id", e.ID, "event_type", e.EventType, "error", err)
			continue
		}
		if _, err := r.pool.Exec(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, e.ID); err != nil {
			return err
		}
	}

	return nil
}
