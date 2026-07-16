-- Platform admins - the account(s) allowed to create an Organization
-- once at least one already exists (see internal/tenancy/application/
-- create_organization.go's own bootstrap comment: the very first
-- Organization ever is exempt, and its creator is granted this flag as
-- a side effect). users has no RLS (it's platform-global, same as
-- api_keys) - a plain column, no policy work needed.
ALTER TABLE users ADD COLUMN is_platform_admin boolean NOT NULL DEFAULT false;
