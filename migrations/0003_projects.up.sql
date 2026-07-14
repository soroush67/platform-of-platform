-- Project: the aggregate the Tenancy table map (docs/architecture/05-database.md
-- SS2) always listed under Organization/Team but 0001_init.up.sql
-- deliberately didn't build yet - the walking skeleton's first two
-- migrations covered exactly the Identity/RBAC bootstrap loop (org,
-- user, role, membership) needed to prove auth end-to-end; Project sat
-- out until something actually needed it.

CREATE TABLE projects (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    name            text NOT NULL,
    slug            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- "projects.slug unique *within org*" (docs/architecture/05-database.md
    -- SS2) - a plain composite UNIQUE, no partial-index trick needed like
    -- roles' builtin-vs-org split: organization_id is NOT NULL on every
    -- row here, unlike roles.organization_id.
    UNIQUE (organization_id, slug)
);

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects FORCE ROW LEVEL SECURITY;
CREATE POLICY projects_isolation ON projects
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT, UPDATE ON projects TO platform_app;
