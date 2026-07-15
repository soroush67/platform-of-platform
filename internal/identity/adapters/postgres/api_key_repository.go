package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/identity/domain"
)

// APIKeyRepository - no set_config/RLS scoping anywhere in this type,
// deliberately: api_keys has no RLS at all (migrations/0017's own
// comment on why - authenticating a presented key is inherently a
// cross-org lookup-by-hash, before any organization_id is known).
// organization_id is still passed explicitly and filtered on in plain
// WHERE clauses wherever a caller already knows it (Revoke), the same
// "RLS doesn't apply here, so the application layer enforces the
// boundary explicitly" posture as every outbox_events query in this
// codebase.
type APIKeyRepository struct {
	pool *pgxpool.Pool
}

func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{pool: pool}
}

func (r *APIKeyRepository) Create(ctx context.Context, organizationID string, key *domain.APIKey) error {
	scopes, err := json.Marshal(key.Scopes)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO api_keys (id, organization_id, owner_type, owner_id, name, key_hash, scopes, expires_at, last_used_at, revoked_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		key.ID, organizationID, key.OwnerType, key.OwnerID, key.Name, key.KeyHash, scopes, key.ExpiresAt, key.LastUsedAt, key.RevokedAt, key.CreatedAt,
	)
	return err
}

// GetByHash is the real authentication-path lookup
// (httpserver.RequireAuth's own API-key resolver, wired in
// cmd/control-plane/main.go) - returns domain.ErrAPIKeyInvalid for both
// "no such key" and "key exists but is expired/revoked," the same
// non-distinguishing posture RefreshTokenRepository already established
// (a stolen, already-revoked key shouldn't get a different error than a
// made-up one).
func (r *APIKeyRepository) GetByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	var k domain.APIKey
	var scopesRaw []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, owner_type, owner_id, name, key_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		 FROM api_keys WHERE key_hash = $1`,
		keyHash,
	).Scan(&k.ID, &k.OwnerType, &k.OwnerID, &k.Name, &k.KeyHash, &scopesRaw, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt, &k.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrAPIKeyInvalid
		}
		return nil, err
	}
	if err := json.Unmarshal(scopesRaw, &k.Scopes); err != nil {
		return nil, err
	}
	return &k, nil
}

// TouchLastUsed is best-effort bookkeeping (called from the
// authentication path itself) - real "when was this key last actually
// used" tracking, the same field docs/architecture/03-domain-model.md
// §3 names for APIKey.
func (r *APIKeyRepository) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	return err
}

// Revoke implements `DELETE /orgs/{org}/service-accounts/{sa}/api-keys/{key}` -
// organization_id filtered explicitly in the WHERE clause (no RLS on
// this table to do it implicitly).
func (r *APIKeyRepository) Revoke(ctx context.Context, organizationID, keyID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND organization_id = $2 AND revoked_at IS NULL`, keyID, organizationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAPIKeyInvalid
	}
	return nil
}
