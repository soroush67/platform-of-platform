-- Workspace & Environment context (docs/architecture/03-domain-model.md
-- §5, docs/architecture/05-database.md table map). Both tables carry
-- organization_id even though their "real" parent is project_id - same
-- denormalize-for-RLS reasoning already applied to role_bindings in
-- 0001_init.up.sql: every tenant-owned table needs a real organization_id
-- column for RLS to filter on (docs/architecture/05-database.md §1),
-- not a derived value looked up through a join.

CREATE TABLE environments (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   uuid NOT NULL REFERENCES organizations (id),
    project_id        uuid NOT NULL REFERENCES projects (id),
    name              text NOT NULL,
    promotion_rank    int NOT NULL DEFAULT 0,
    requires_approval bool NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    -- Not explicitly stated in Stage 3 §5, but a reasonable inferred
    -- invariant (same call already made for projects.slug/organizations.slug):
    -- two environments named "production" in the same project would make
    -- the promotion-flow UI ambiguous about which one a Run promotes into.
    UNIQUE (project_id, name)
);

ALTER TABLE environments ENABLE ROW LEVEL SECURITY;
ALTER TABLE environments FORCE ROW LEVEL SECURITY;
CREATE POLICY environments_isolation ON environments
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE ON environments TO platform_app;

CREATE TABLE workspaces (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id        uuid NOT NULL REFERENCES organizations (id),
    project_id             uuid NOT NULL REFERENCES projects (id),
    -- Nullable: "a workspace can exist outside any environment, e.g. a
    -- scratch/dev workspace" (docs/architecture/03-domain-model.md §5).
    -- No FK to environments - a real one would be natural once the
    -- Environment side of this migration is exercised by real workspace
    -- rows; deferred rather than added speculatively, same as the
    -- vcs_link_id/current_state_version_id columns below.
    environment_id         uuid,
    name                   text NOT NULL,
    -- Closed set (docs/architecture/03-domain-model.md §5) - CHECK makes
    -- an invalid engine a schema-level impossibility, same "closed set ->
    -- real constraint" reasoning already applied to users.auth_source.
    execution_engine       text NOT NULL CHECK (execution_engine IN
        ('terraform', 'opentofu', 'ansible', 'helm', 'compose', 'packer', 'kubespray', 'kubernetes')),
    -- Both nullable, both FK-less on purpose: GitOps and State are real
    -- future contexts (docs/architecture/03-domain-model.md §5/§9) that
    -- don't exist yet in this codebase - these columns exist now because
    -- Workspace's own shape needs them, but they can't reference tables
    -- that aren't built yet. Adding the FK is that context's own
    -- migration's job when it lands.
    vcs_link_id            uuid,
    current_state_version_id uuid,
    -- docs/architecture/05-database.md table map: "workspaces.lock_status
    -- as (locked bool, locked_by_run_id uuid nullable)" - locked_by_run_id
    -- is FK-less for the same reason as vcs_link_id: the Execution
    -- context's runs table doesn't exist yet.
    locked                 bool NOT NULL DEFAULT false,
    locked_by_run_id       uuid,
    created_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

ALTER TABLE workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspaces FORCE ROW LEVEL SECURITY;
CREATE POLICY workspaces_isolation ON workspaces
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE ON workspaces TO platform_app;
