ALTER TABLE variables DROP CONSTRAINT IF EXISTS variables_value_xor_secret_ref;
ALTER TABLE variables DROP COLUMN IF EXISTS secret_path;
ALTER TABLE variables DROP COLUMN IF EXISTS secret_mount_id;
ALTER TABLE variables ALTER COLUMN value SET NOT NULL;

DROP TABLE IF EXISTS secret_mounts;
