-- Local auth needs somewhere to store the credential. Nullable: only
-- auth_source='local' users ever have one - oidc/saml/ldap users
-- authenticate via their IdP and never touch this column
-- (docs/architecture/19-integrations.md §2).
-- Already GRANTed to platform_app on the whole users table in
-- 0001_init.up.sql - no new grant needed for this column.
ALTER TABLE users ADD COLUMN password_hash text;
