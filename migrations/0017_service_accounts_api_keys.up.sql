-- Closes the "no API Keys/ServiceAccounts" gap
-- (docs/architecture/13-module-identity-rbac-tenancy.md §2):
--   POST /orgs/{org}/service-accounts
--   POST /orgs/{org}/service-accounts/{sa}/api-keys
--   DELETE /orgs/{org}/service-accounts/{sa}/api-keys/{key}
--
-- ServiceAccount is org-scoped (unlike User, which is platform-global) -
-- real RLS the same as every other tenant-facing table since 0001_init.
CREATE TABLE service_accounts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

ALTER TABLE service_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE service_accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY service_accounts_isolation ON service_accounts
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

-- api_keys deliberately has NO RLS - same reasoning as `users` and
-- `outbox_events` (migrations/0001_init.up.sql, 0007_outbox_audit.up.sql):
-- authenticating a presented API key is inherently a lookup-by-hash
-- *before* the request's organization is known at all (the same
-- chicken-and-egg RLS would create for JWT auth too, if a JWT's claims
-- carried an org id instead of resolving it per-request from the URL,
-- which is exactly why they don't - see auth.IssueAccessToken's own
-- comment). organization_id is still a real column here (needed once
-- the key IS resolved, e.g. to scope the ServiceAccount CRUD endpoints),
-- just not RLS-enforced on this table - the request handler's own
-- httpserver.RequireAuth resolves the key to a subject id first, then
-- every *subsequent* org-scoped query in the request goes through the
-- normal RLS-protected tables as usual.
CREATE TABLE api_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    owner_type      text NOT NULL CHECK (owner_type IN ('user', 'service_account')),
    owner_id        uuid NOT NULL,
    name            text NOT NULL,
    key_hash        text NOT NULL UNIQUE,
    -- scopes (docs/architecture/13-module-identity-rbac-tenancy.md §2:
    -- "optional narrowing below the owner's own RBAC grants") - stored
    -- and returned real, validated against the same Permission enum at
    -- creation time as a custom Role's own permissions; the *runtime*
    -- intersection-with-RBAC enforcement is a further, real, named gap
    -- (see internal/identity/application/create_api_key.go's own
    -- comment) - not silently claimed as fully wired up.
    scopes          jsonb NOT NULL DEFAULT '[]',
    expires_at      timestamptz,
    last_used_at    timestamptz,
    revoked_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX api_keys_key_hash ON api_keys (key_hash);
CREATE INDEX api_keys_owner ON api_keys (owner_type, owner_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON service_accounts, api_keys TO platform_app;
