-- ComposeFile<->Project many-to-many link (operator ask: a ComposeFile
-- should be linkable to multiple Projects at once, add/remove gated by
-- compose_file:manage). Same shape as compose_file_networks/
-- compose_file_volumes (migration 0019) - a pure junction table, the
-- ComposeFile itself stays organization-scoped, this only records which
-- Projects it's currently linked into.
CREATE TABLE compose_file_projects (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    compose_file_id  uuid NOT NULL REFERENCES compose_files (id) ON DELETE CASCADE,
    project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    UNIQUE (compose_file_id, project_id)
);
ALTER TABLE compose_file_projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_file_projects FORCE ROW LEVEL SECURITY;
CREATE POLICY compose_file_projects_isolation ON compose_file_projects
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, DELETE ON compose_file_projects TO platform_app;
