// Package domain holds the Audit context's pure Go types
// (docs/architecture/03-domain-model.md §15).
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrForbidden = errors.New("forbidden")

// Entry is append-only by construction, not just convention - there is
// no Update/Delete method on this type, and the database role backing
// this context's repository has no UPDATE/DELETE grant on audit_entries
// at all (migrations/0007_outbox_audit.up.sql). "Populated *exclusively*
// by subscribing to every other context's domain events - no context
// ever calls into Audit directly" (docs/architecture/03-domain-model.md
// §15) - NewEntry below is only ever called from
// application.RecordFromEvent, never from another context's own code.
type Entry struct {
	ID             string
	OrganizationID string
	Actor          string // real user id, or the literal "system"
	Action         string
	TargetType     string
	TargetID       string
	Metadata       map[string]any
	CreatedAt      time.Time
}

func NewEntry(organizationID, actor, action, targetType, targetID string, metadata map[string]any) *Entry {
	return &Entry{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Actor:          actor,
		Action:         action,
		TargetType:     targetType,
		TargetID:       targetID,
		Metadata:       metadata,
		CreatedAt:      time.Now().UTC(),
	}
}
