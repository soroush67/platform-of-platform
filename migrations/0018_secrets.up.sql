-- Secrets context (docs/architecture/11-module-secrets-state.md §1) -
-- SecretMount is the real aggregate; SecretReference (§2) is a value
-- object embedded inside Variable, never its own top-level resource -
-- there is deliberately no secrets table beyond this one.
--
-- Only backend_type = 'vault' is actually implemented (the Vault Go SDK
-- adapter, internal/secrets/adapters/vault) - the other three values are
-- modeled in this CHECK constraint (matching the doc's own REST API
-- shape) but rejected by CreateSecretMountService with a clear
-- "not yet implemented" validation error, the same "modeled fully,
-- implemented partially, flagged rather than silently narrowed" posture
-- already applied to runs.trigger/runs.status elsewhere in this schema.
CREATE TABLE secret_mounts (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     uuid NOT NULL REFERENCES organizations (id),
    name                text NOT NULL,
    backend_type        text NOT NULL CHECK (backend_type IN ('vault', 'aws_secrets_manager', 'azure_keyvault', 'gcp_secret_manager')),
    address             text NOT NULL,
    -- AppRole role_id is not itself sensitive (it's a stable identifier,
    -- not a credential - Vault's own AppRole docs are explicit about
    -- this), so it's stored plain. secret_id is the real credential -
    -- see docs/architecture/11-module-secrets-state.md §1's own
    -- "encrypted at rest... envelope-encryption scheme" and
    -- internal/platform/envelope's own doc comment for the scheme
    -- itself (BLAKE2b-derived per-record key, AES-GCM). Three columns,
    -- not one blob, so the salt/nonce never need to be packed/unpacked
    -- from a single bytea by hand.
    role_id             text NOT NULL,
    encrypted_secret_id bytea NOT NULL,
    secret_id_nonce     bytea NOT NULL,
    secret_id_salt      bytea NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

ALTER TABLE secret_mounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE secret_mounts FORCE ROW LEVEL SECURITY;
CREATE POLICY secret_mounts_isolation ON secret_mounts
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT ON secret_mounts TO platform_app;

-- The other half of this migration: variables.value becomes nullable,
-- and a Variable can instead carry a SecretReference (mount + path) -
-- closing the exact gap 0006_variables.up.sql's own comment named
-- ("The Secrets context doesn't exist in this codebase yet... A
-- secret_ref column... is that future context's own migration to add").
-- No FK from variables.secret_mount_id straight into secret_mounts:
-- CockroachDB's RLS is per-table, and a variable in one scope
-- referencing a mount is already validated at the application layer
-- (CreateVariableService's own SecretMountChecker port, same "validate
-- the reference before writing" posture as every other cross-context
-- check in this codebase) - an FK here would need the two tables'
-- RLS policies to agree at the SQL level, which they can't, the same
-- reasoning role_bindings.scope_id/role_id already established for
-- polymorphic and cross-scope references.
ALTER TABLE variables ALTER COLUMN value DROP NOT NULL;
ALTER TABLE variables ADD COLUMN secret_mount_id uuid;
ALTER TABLE variables ADD COLUMN secret_path text;
ALTER TABLE variables ADD CONSTRAINT variables_value_xor_secret_ref CHECK (
    (value IS NOT NULL AND secret_mount_id IS NULL AND secret_path IS NULL) OR
    (value IS NULL AND secret_mount_id IS NOT NULL AND secret_path IS NOT NULL)
);
