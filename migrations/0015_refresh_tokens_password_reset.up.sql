-- Identity gaps named directly: no refresh token (access tokens are a
-- hard 15-minute TTL, docs/architecture's own "short-lived, no
-- revocation list" tradeoff - the real gap is there was no way to
-- extend a session without a full re-login) and no password reset.
--
-- Neither table gets organization_id/RLS - User is platform-global
-- (docs/architecture/03-domain-model.md §3), matching the `users` table
-- itself (migrations/0001_init.up.sql), not tenant data.
--
-- Both store a hash, never the plaintext token - the "shown once, never
-- again, nothing server-side ever holds the plaintext" posture this
-- operator's own vault-ha project established for unseal keys, already
-- reused this session for Idempotency-Key's own comment.
CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id),
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The exact lookup RefreshAccessToken does on every call: hash the
-- presented token, find the row, check revoked_at/expires_at.
CREATE INDEX refresh_tokens_token_hash ON refresh_tokens (token_hash);
CREATE INDEX refresh_tokens_user_id ON refresh_tokens (user_id);

CREATE TABLE password_reset_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id),
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_token_hash ON password_reset_tokens (token_hash);

GRANT SELECT, INSERT, UPDATE ON refresh_tokens, password_reset_tokens TO platform_app;
