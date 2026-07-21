ALTER TABLE machines DROP CONSTRAINT machines_credential_shape;
ALTER TABLE machines DROP COLUMN credential_salt;
ALTER TABLE machines DROP COLUMN credential_nonce;
ALTER TABLE machines DROP COLUMN encrypted_credential;
ALTER TABLE machines DROP COLUMN credential_storage;
ALTER TABLE machines ALTER COLUMN credential_path SET NOT NULL;
ALTER TABLE machines ALTER COLUMN credential_mount_id SET NOT NULL;
