-- Idempotency-Key support (docs/architecture/04-api-design.md §5):
-- "Every state-mutating endpoint that a CI script or webhook handler
-- might legitimately retry accepts an Idempotency-Key header... first
-- request with a given key executes and caches (key -> response) for
-- 24h; a repeated request with the same key inside that window returns
-- the cached response without re-executing." Cross-cutting
-- infrastructure (internal/platform/idempotency), not owned by any one
-- bounded context - same category as outbox_events.
CREATE TABLE idempotency_keys (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    uuid NOT NULL REFERENCES organizations (id),
    -- Scoped per (org, user, key) - the same key string from two
    -- different users (or the same user acting in two different orgs)
    -- must never collide; a client-generated key only needs to be
    -- unique within its own actual calling context.
    requesting_user_id uuid NOT NULL,
    idempotency_key    text NOT NULL,
    response_status    int NOT NULL,
    response_body      bytea NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, requesting_user_id, idempotency_key)
);

-- The 24h staleness check (Store.Get filters on created_at) is the hot
-- path this index serves - rows past the window aren't actively
-- deleted (a real, deferred gap: this table grows by one row per
-- distinct idempotency key ever used, unbounded but bounded by actual
-- client-provided-key volume, same posture already accepted for
-- Registry.runToWorker in the gRPC adapter).
CREATE INDEX idempotency_keys_created_at ON idempotency_keys (created_at);

ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY idempotency_keys_isolation ON idempotency_keys
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

GRANT SELECT, INSERT ON idempotency_keys TO platform_app;
