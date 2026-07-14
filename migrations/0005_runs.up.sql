-- Execution context (docs/architecture/03-domain-model.md §6) - "the
-- core workflow." organization_id denormalized for RLS, same pattern as
-- every tenant-owned table since 0001.

CREATE TABLE runs (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    workspace_id     uuid NOT NULL REFERENCES workspaces (id),
    trigger          text NOT NULL CHECK (trigger IN ('manual', 'vcs_push', 'vcs_pr', 'scheduled', 'api')),
    -- "user_id or 'system' for scheduled" (Stage 3 §6) - a real uuid
    -- string or the literal "system", never both, so this can't be a
    -- clean FK to users(id) the way most other actor columns in this
    -- schema are. Only 'manual'/'api' triggers exist in this slice's own
    -- code (no scheduler built yet), so every row today is a real user id,
    -- but the column stays shaped for the 'system' case Stage 3 names.
    triggered_by     text NOT NULL,
    status           text NOT NULL CHECK (status IN
        ('queued', 'planning', 'planned', 'policy_check', 'awaiting_approval',
         'applying', 'applied', 'failed', 'errored', 'canceled')),
    plan_output_ref  text,
    apply_output_ref text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    started_at       timestamptz,
    finished_at      timestamptz
);

-- "(workspace_id, status, created_at desc) - the Stage 4 cursor-
-- pagination query's exact shape" (docs/architecture/05-database.md
-- table map, Execution row). No partial "is this workspace locked"
-- index yet - that hot-path check reads workspaces.locked directly
-- (see TryLock in workspace's own postgres adapter), not this table.
CREATE INDEX runs_workspace_status_created ON runs (workspace_id, status, created_at DESC);

ALTER TABLE runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE runs FORCE ROW LEVEL SECURITY;
CREATE POLICY runs_isolation ON runs
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE ON runs TO platform_app;
