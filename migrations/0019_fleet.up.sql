-- Fleet context (internal/fleet) - ported from a separate Python/FastAPI
-- product (/home/soroush/compose-platform) that centrally manages/deploys
-- docker-compose files across multiple remote machines over SSH. Phase 1
-- only: Machines, Networks/Volumes catalogs, ComposeFiles (upload only,
-- no GitLab ingestion), per-ComposeFile Variables, deploy Operations.
-- Groups, a ChangeRequest approval workflow, GitLab ingestion, and
-- NotificationSettings are all deliberately deferred to a later phase.

-- Machine's SSH credential is a SecretReference (mount + path) into the
-- EXISTING Secrets/Vault mechanism (secret_mounts, migration 0018) -
-- resolved live at connect time, never persisted here. No FK into
-- secret_mounts, same reasoning variables.secret_mount_id already
-- established: RLS is per-table, the reference is validated at the
-- application layer (SecretMountChecker), not enforced at the SQL level.
CREATE TABLE machines (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id      uuid NOT NULL REFERENCES organizations (id),
    name                 text NOT NULL,
    host                 text NOT NULL,
    ssh_port             int NOT NULL DEFAULT 22,
    ssh_user             text NOT NULL,
    credential_type      text NOT NULL CHECK (credential_type IN ('ssh_key', 'ssh_password')),
    credential_mount_id  uuid NOT NULL,
    credential_path      text NOT NULL,
    deploy_base_path     text NOT NULL,
    connection_status    text NOT NULL DEFAULT 'unknown' CHECK (connection_status IN ('unknown', 'online', 'unreachable')),
    docker_status        text NOT NULL DEFAULT 'unknown' CHECK (docker_status IN ('unknown', 'ok', 'missing', 'error')),
    last_checked_at      timestamptz,
    archived             boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
CREATE INDEX machines_org_archived ON machines (organization_id, archived);

ALTER TABLE machines ENABLE ROW LEVEL SECURITY;
ALTER TABLE machines FORCE ROW LEVEL SECURITY;
CREATE POLICY machines_isolation ON machines
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON machines TO platform_app;

-- Networks/Volumes: simple admin-managed catalogs, referenced (not
-- duplicated) by ComposeFiles via the junction tables below.
CREATE TABLE networks (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    name             text NOT NULL,
    external         boolean NOT NULL DEFAULT false,
    created_by       uuid NOT NULL REFERENCES users (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
ALTER TABLE networks ENABLE ROW LEVEL SECURITY;
ALTER TABLE networks FORCE ROW LEVEL SECURITY;
CREATE POLICY networks_isolation ON networks
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, DELETE ON networks TO platform_app;

CREATE TABLE volumes (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    name             text NOT NULL,
    host_path        text NOT NULL,
    created_by       uuid NOT NULL REFERENCES users (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
ALTER TABLE volumes ENABLE ROW LEVEL SECURITY;
ALTER TABLE volumes FORCE ROW LEVEL SECURITY;
CREATE POLICY volumes_isolation ON volumes
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, DELETE ON volumes TO platform_app;

-- ComposeFile: the Python product's own "Profile", renamed there before
-- this port ever started. is_global marks the org's fallback ComposeFile
-- for variable resolution - the Python original is single-tenant and
-- seeds exactly one global row process-wide at startup; this codebase is
-- multi-tenant, so "at most one global ComposeFile per Organization" is
-- enforced here via a partial unique index instead of a startup seed.
-- No GitLab source fields - GitLab ingestion is a deferred phase.
CREATE TABLE compose_files (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   uuid NOT NULL REFERENCES organizations (id),
    name              text NOT NULL,
    is_global         boolean NOT NULL DEFAULT false,
    compose_content   text NOT NULL DEFAULT '',
    created_by        uuid NOT NULL REFERENCES users (id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
CREATE UNIQUE INDEX compose_files_one_global_per_org ON compose_files (organization_id) WHERE is_global;

ALTER TABLE compose_files ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_files FORCE ROW LEVEL SECURITY;
CREATE POLICY compose_files_isolation ON compose_files
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, UPDATE ON compose_files TO platform_app;

CREATE TABLE compose_file_networks (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    compose_file_id  uuid NOT NULL REFERENCES compose_files (id) ON DELETE CASCADE,
    network_id       uuid NOT NULL REFERENCES networks (id) ON DELETE CASCADE,
    UNIQUE (compose_file_id, network_id)
);
ALTER TABLE compose_file_networks ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_file_networks FORCE ROW LEVEL SECURITY;
CREATE POLICY compose_file_networks_isolation ON compose_file_networks
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, DELETE ON compose_file_networks TO platform_app;

CREATE TABLE compose_file_volumes (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    compose_file_id  uuid NOT NULL REFERENCES compose_files (id) ON DELETE CASCADE,
    volume_id        uuid NOT NULL REFERENCES volumes (id) ON DELETE CASCADE,
    container_path   text NOT NULL,
    UNIQUE (compose_file_id, volume_id)
);
ALTER TABLE compose_file_volumes ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_file_volumes FORCE ROW LEVEL SECURITY;
CREATE POLICY compose_file_volumes_isolation ON compose_file_volumes
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
-- UPDATE (not just SELECT/INSERT/DELETE) - AttachVolume's own
-- ON CONFLICT (compose_file_id, volume_id) DO UPDATE SET container_path
-- (re-attaching with a different container_path updates it in place)
-- needs real UPDATE privilege, unlike compose_file_networks' own
-- ON CONFLICT DO NOTHING above.
GRANT SELECT, INSERT, UPDATE, DELETE ON compose_file_volumes TO platform_app;

-- Named fleet_variables, not variables - that table name already belongs
-- to internal/variables. ComposeFileID is always a real id (no nullable-
-- FK-means-global special case the Python schema has) - a "global"
-- variable is just one created against the org's own is_global
-- ComposeFile, enabled by the per-org global design above.
--
-- var_type=secret variables resolve through the SAME Secrets/Vault
-- SecretReference mechanism as Machine credentials above and as
-- internal/variables' own secret-typed Variables - not the Python
-- original's own per-record AES-GCM/BLAKE2b envelope (value_encrypted
-- column). That column is deliberately not ported.
CREATE TABLE fleet_variables (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    uuid NOT NULL REFERENCES organizations (id),
    compose_file_id    uuid NOT NULL REFERENCES compose_files (id) ON DELETE CASCADE,
    key                text NOT NULL,
    var_type           text NOT NULL CHECK (var_type IN ('kv', 'secret', 'env', 'file_template', 'config_file')),
    value              text,
    secret_mount_id    uuid,
    secret_path        text,
    file_target_path   text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (compose_file_id, key),
    CHECK (
        (value IS NOT NULL AND secret_mount_id IS NULL AND secret_path IS NULL) OR
        (value IS NULL AND secret_mount_id IS NOT NULL AND secret_path IS NOT NULL)
    ),
    CHECK (var_type NOT IN ('file_template', 'config_file') OR file_target_path IS NOT NULL)
);
ALTER TABLE fleet_variables ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleet_variables FORCE ROW LEVEL SECURITY;
CREATE POLICY fleet_variables_isolation ON fleet_variables
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON fleet_variables TO platform_app;

-- Operations: immutable history once created (like runs/audit_entries) -
-- no DELETE grant. status adds a real 'queued' state the Python original
-- didn't need (its Celery task picks up a just-inserted row near-
-- instantly; Go's own DeployExecutor is a ticker-poll Runnable with a
-- genuine queued window between INSERT and the next poll tick).
CREATE TABLE operations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id),
    compose_file_id  uuid NOT NULL REFERENCES compose_files (id),
    machine_id       uuid NOT NULL REFERENCES machines (id),
    operation_type   text NOT NULL CHECK (operation_type IN ('deploy', 'up', 'down', 'restart', 'pull', 'build', 'stop', 'start', 'remove')),
    status           text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'success', 'failed')),
    triggered_by     uuid NOT NULL REFERENCES users (id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    started_at       timestamptz,
    finished_at      timestamptz,
    exit_code        int,
    output           text NOT NULL DEFAULT ''
);
CREATE INDEX operations_org_status_created ON operations (organization_id, status, created_at DESC);
CREATE INDEX operations_compose_file ON operations (compose_file_id, created_at DESC);
CREATE INDEX operations_machine ON operations (machine_id, created_at DESC);

ALTER TABLE operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE operations FORCE ROW LEVEL SECURITY;
CREATE POLICY operations_isolation ON operations
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);
GRANT SELECT, INSERT, UPDATE ON operations TO platform_app;
