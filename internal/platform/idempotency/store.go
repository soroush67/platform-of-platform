// Package idempotency implements docs/architecture/04-api-design.md
// §5's Idempotency-Key mechanism - cross-cutting HTTP infrastructure
// (internal/platform, like httpserver/auth/outbox), not owned by any
// one bounded context: any state-mutating endpoint that's safe to
// re-execute given a fresh idempotency key can opt into Middleware,
// though this codebase only wires it into the one endpoint the doc
// itself names as the concrete case that matters most - POST
// .../workspaces/{workspace}/runs ("did my apply run twice" being a
// much worse failure mode here than for a typical CRUD endpoint).
package idempotency

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Window is the idempotency cache lifetime (docs/architecture/
// 04-api-design.md §5's "24h") - the single source of truth both Get's
// own staleness filter and Reaper's cleanup sweep (reaper.go) compute
// their cutoff from. Previously a bare '24 hours' literal baked
// straight into Get's SQL (Duration.String() - "24h0m0s" - isn't valid
// Postgres/CockroachDB interval syntax, so it couldn't just be
// Sprintf'd in); keeping it as one Go constant and passing the computed
// cutoff as a real parameter is what makes it safe for Reaper to exist
// at all - a second, independently-typed copy of "24 hours" in two
// files would risk drifting out of sync, and Reaper deleting a row Get
// would still consider fresh would be a real, silent user-facing bug
// (a legitimate retry inside the window suddenly re-executing).
const Window = 24 * time.Hour

// CachedResponse is what Get returns for a key that was already used -
// exactly what the first request's handler actually produced, replayed
// verbatim rather than the operation running a second time.
type CachedResponse struct {
	Status int
	Body   []byte
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Get returns (nil, nil) if this key has never been used, or has aged
// out of Window - rows past the window aren't deleted by this method,
// just ignored by this query's own created_at filter; Reaper is what
// actually deletes them (see its own doc comment).
func (s *Store) Get(ctx context.Context, organizationID, userID, key string) (*CachedResponse, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-Window)
	var resp CachedResponse
	err = tx.QueryRow(ctx,
		`SELECT response_status, response_body FROM idempotency_keys
		 WHERE organization_id = $1 AND requesting_user_id = $2 AND idempotency_key = $3
		   AND created_at > $4`,
		organizationID, userID, key, cutoff,
	).Scan(&resp.Status, &resp.Body)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &resp, tx.Commit(ctx)
}

// Save records the response for future replay. ON CONFLICT DO NOTHING
// is the deliberate, documented boundary of this implementation's
// concurrency guarantee: two genuinely concurrent requests using the
// same fresh key can both miss Get() and both actually execute the
// underlying operation (a real, narrower race than the sequential
// "client timed out and retried after the first one already finished"
// scenario this store fully solves) - whichever one's Save reaches
// Postgres/CockroachDB first becomes canonical for any *future* retry,
// but this method doesn't stop a truly simultaneous duplicate from
// happening once. Closing that fully would need a claim-the-key-before-
// executing pattern (INSERT a pending row first, let the UNIQUE
// constraint itself block a concurrent second execution) - the same
// "at-least-once, not exactly-once" honesty already central to this
// codebase's own Outbox Relay, applied here instead of silently
// claiming a stronger guarantee than what's actually implemented.
func (s *Store) Save(ctx context.Context, organizationID, userID, key string, status int, body []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO idempotency_keys (organization_id, requesting_user_id, idempotency_key, response_status, response_body)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (organization_id, requesting_user_id, idempotency_key) DO NOTHING`,
		organizationID, userID, key, status, body,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
