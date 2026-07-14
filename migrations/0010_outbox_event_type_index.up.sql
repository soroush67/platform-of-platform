-- The Stale Run Reaper (docs/architecture/07-module-execution.md §3's
-- own named "single most commonly-missed piece in a first-pass
-- execution-engine design") reads outbox_events directly for
-- RunApplying rows older than a threshold, regardless of published_at -
-- the existing outbox_events_unpublished index (0007_outbox_audit.up.sql)
-- is partial (WHERE published_at IS NULL) and stops covering a row the
-- moment the Audit/Dispatch Relay marks it published, which is always,
-- quickly. A real, non-partial index on (event_type, occurred_at) is
-- what the Reaper's query actually needs.
CREATE INDEX outbox_events_type_occurred ON outbox_events (event_type, occurred_at);
