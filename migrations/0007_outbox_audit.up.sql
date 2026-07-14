-- Events (docs/architecture/06-events.md) + Audit (docs/architecture/
-- 03-domain-model.md §15). First real implementation of the
-- Transactional Outbox pattern in this codebase: a domain write and its
-- event both commit (or both roll back) in the same transaction - see
-- internal/platform/outbox/outbox.go for the write side, internal/
-- platform/outbox/relay.go for the dispatch side.

CREATE TABLE outbox_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    event_type      text NOT NULL,
    payload         jsonb NOT NULL,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    published_at    timestamptz
);

-- The Relay's own hot-path query: unpublished rows, oldest first.
CREATE INDEX outbox_events_unpublished ON outbox_events (occurred_at) WHERE published_at IS NULL;

GRANT SELECT, INSERT, UPDATE ON outbox_events TO platform_app;

-- Deliberately NO RLS on this table, breaking from every other tenant
-- table's pattern since 0001 - and deliberately, not by oversight. The
-- Outbox Relay is a system-internal background process (docs/
-- architecture/18-backend-structure.md §4's Runnable pattern), not a
-- per-request handler acting on behalf of one authenticated Principal -
-- it has no "current org" to scope to, because its whole job is
-- processing events *across every org* in one pass. No HTTP handler in
-- this codebase ever queries outbox_events directly either (only the
-- Relay reads it, and only audit_entries - the derived, genuinely
-- tenant-facing table below - is ever exposed through the API), so
-- there's no tenant-facing read path this table's lack of RLS could
-- leak through.

CREATE TABLE audit_entries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    -- Same "real user id or the literal 'system'" shape as
    -- runs.triggered_by (migrations/0005_runs.up.sql) - not a clean FK
    -- for the same reason.
    actor           text NOT NULL,
    action          text NOT NULL,
    target_type     text NOT NULL,
    target_id       text NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- "(organization_id, created_at desc), (target_type, target_id)"
-- (docs/architecture/05-database.md table map, Audit row).
CREATE INDEX audit_entries_org_created ON audit_entries (organization_id, created_at DESC);
CREATE INDEX audit_entries_target ON audit_entries (target_type, target_id);

ALTER TABLE audit_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_entries_isolation ON audit_entries
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

-- "no updated_at, no update/delete privilege granted to the app's DB
-- role at all - enforced at the database-permission level, not just
-- 'the code never calls UPDATE,' so a bug (or a compromised app
-- process) still can't rewrite history" (docs/architecture/
-- 05-database.md table map, Audit row) - SELECT and INSERT only, no
-- UPDATE, no DELETE, matched by internal/audit's own repository never
-- even having Update/Delete methods to call.
GRANT SELECT, INSERT ON audit_entries TO platform_app;
