-- Variables context (docs/architecture/03-domain-model.md §7). scope_id
-- is a discriminated pointer (organization_id | project_id |
-- environment_id | workspace_id depending on scope_type), not a real
-- FK - Postgres/CockroachDB can't FK-constrain a polymorphic column,
-- same reasoning already applied to role_bindings.scope_id in
-- 0001_init.up.sql. organization_id is still a real, non-polymorphic FK,
-- carried for RLS the same way every tenant table since 0001 does.

CREATE TABLE variables (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    scope_type      text NOT NULL CHECK (scope_type IN ('organization', 'project', 'environment', 'workspace')),
    scope_id        uuid NOT NULL,
    key             text NOT NULL,
    category        text NOT NULL CHECK (category IN ('env_var', 'engine_var', 'file_template')),
    sensitivity     text NOT NULL CHECK (sensitivity IN ('plain', 'sensitive')),
    -- Stage 3 §7: "value (plain text) OR secret_ref (-> Secrets context,
    -- mutually exclusive with value)". The Secrets context doesn't exist
    -- in this codebase yet (Stage 8, not built) - only the `value` path
    -- is supported here. A secret_ref column (nullable, with a CHECK
    -- that exactly one of value/secret_ref is set) is that future
    -- context's own migration to add, not speculatively reserved here.
    value           text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- "(scope_type, scope_id, key) unique - two variables can't collide
    -- at the same scope" (docs/architecture/05-database.md table map,
    -- Variables row) - this composite UNIQUE also gives the exact index
    -- the resolution cascade's per-scope lookup needs.
    UNIQUE (scope_type, scope_id, key)
);

ALTER TABLE variables ENABLE ROW LEVEL SECURITY;
ALTER TABLE variables FORCE ROW LEVEL SECURITY;
CREATE POLICY variables_isolation ON variables
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE ON variables TO platform_app;
