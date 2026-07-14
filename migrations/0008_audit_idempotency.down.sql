ALTER TABLE audit_entries DROP CONSTRAINT audit_entries_source_event_unique;
ALTER TABLE audit_entries DROP COLUMN source_event_id;
