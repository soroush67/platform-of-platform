-- Machine credential can now optionally live outside Vault entirely -
-- "local" storage seals the SSH secret directly into this row via the
-- same envelope scheme (internal/platform/envelope) secret_mounts.
-- encrypted_secret_id already uses (migration 0018), so a Machine can be
-- created and connected to with zero live Vault dependency. credential_
-- mount_id/credential_path (the "vault" shape) become nullable - exactly
-- one of the two shapes is populated per row, enforced below.
ALTER TABLE machines ALTER COLUMN credential_mount_id DROP NOT NULL;
ALTER TABLE machines ALTER COLUMN credential_path DROP NOT NULL;

ALTER TABLE machines ADD COLUMN credential_storage text NOT NULL DEFAULT 'vault' CHECK (credential_storage IN ('vault', 'local'));
ALTER TABLE machines ADD COLUMN encrypted_credential bytea;
ALTER TABLE machines ADD COLUMN credential_nonce bytea;
ALTER TABLE machines ADD COLUMN credential_salt bytea;

ALTER TABLE machines ADD CONSTRAINT machines_credential_shape CHECK (
    (credential_storage = 'vault' AND credential_mount_id IS NOT NULL AND credential_path IS NOT NULL AND encrypted_credential IS NULL) OR
    (credential_storage = 'local' AND credential_mount_id IS NULL AND credential_path IS NULL AND encrypted_credential IS NOT NULL AND credential_nonce IS NOT NULL AND credential_salt IS NOT NULL)
);
