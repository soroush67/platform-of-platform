package application

import (
	"context"
	"encoding/json"

	"platform-of-platform/internal/audit/domain"
	"platform-of-platform/internal/platform/outbox"
)

// RecordEntryService.HandleEvent implements outbox.Handler's signature
// (func(ctx, outbox.Event) error) - registered with the Relay in
// main.go. This function IS the "no context ever calls into Audit
// directly" guarantee (docs/architecture/03-domain-model.md §15) made
// real: Tenancy/Execution never import this package at all, they only
// ever call outbox.Write with their own event inside their own
// transaction; this is the only code in the whole system that ever
// turns that into an audit_entries row, and it can't be skipped by a
// context forgetting to call it, because there's nothing to call.
//
// Payload contract: every producer's event payload is expected to carry
// "actor", "target_type", "target_id" keys (see
// tenancy/adapters/postgres's OrganizationRepository.Create and
// execution/adapters/postgres's RunRepository for the two real
// producers so far) - the rest of the payload becomes this entry's
// metadata as-is, redundant actor/target fields included, for
// simplicity over stripping them back out.
type RecordEntryService struct {
	repo AuditEntryRepository
}

func NewRecordEntryService(repo AuditEntryRepository) *RecordEntryService {
	return &RecordEntryService{repo: repo}
}

func (s *RecordEntryService) HandleEvent(ctx context.Context, event outbox.Event) error {
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	actor, _ := payload["actor"].(string)
	targetType, _ := payload["target_type"].(string)
	targetID, _ := payload["target_id"].(string)

	entry := domain.NewEntry(event.OrganizationID, event.ID, actor, event.EventType, targetType, targetID, payload)

	return s.repo.Create(ctx, entry)
}
