-- Closes a real, previously-documented gap: the Outbox Relay delivers
-- at-least-once (internal/platform/outbox/relay.go's own comment), and
-- AuditEntryRepository.Create was a plain INSERT with no way to notice
-- a redelivered event, so a crash between a successful HandleEvent call
-- and the Relay's publish-marking UPDATE would duplicate the audit
-- entry. source_event_id ties every entry back to the exact
-- outbox_events row that produced it - a natural idempotency key, not
-- an invented one - and the UNIQUE constraint is what actually makes a
-- retried delivery a safe no-op (see the repository's own
-- ON CONFLICT (source_event_id) DO NOTHING).
ALTER TABLE audit_entries ADD COLUMN source_event_id uuid NOT NULL;
ALTER TABLE audit_entries ADD CONSTRAINT audit_entries_source_event_unique UNIQUE (source_event_id);
