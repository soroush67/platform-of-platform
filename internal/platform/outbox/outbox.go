// Package outbox is the Transactional Outbox mechanism
// (docs/architecture/06-events.md) - shared infrastructure every
// context's adapters use, not itself a bounded context
// (docs/architecture/18-backend-structure.md §1).
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Write inserts an event row using the CALLER's own transaction - not a
// new one. This is the entire point of the pattern: a repository's
// domain write (e.g. INSERT INTO organizations) and this event both
// commit, or both roll back, atomically, because they're the same
// database transaction. A context that wants to emit an event calls
// this once, from inside the same tx.Begin()/Commit() block its own
// domain write already uses - see tenancy/adapters/postgres's
// OrganizationRepository.Create for the first real example.
func Write(ctx context.Context, tx pgx.Tx, organizationID, eventType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (id, organization_id, event_type, payload, occurred_at) VALUES ($1, $2, $3, $4, now())`,
		uuid.NewString(), organizationID, eventType, encoded,
	)
	return err
}

// Event is what a Relay subscriber (Handler) actually receives - the
// raw, already-committed row, read back after the fact, deliberately
// not the same Go value the writer had in memory (the Relay may run
// in a different process someday, or simply after a restart - it only
// ever has what's durably in the table to work from).
type Event struct {
	ID             string
	OrganizationID string
	EventType      string
	Payload        []byte
	OccurredAt     time.Time
}
