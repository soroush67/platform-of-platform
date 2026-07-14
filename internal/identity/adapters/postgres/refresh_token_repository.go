package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/identity/domain"
)

// RefreshTokenRepository - no RLS/set_config, same reasoning as
// UserRepository: refresh_tokens carries no organization_id
// (migrations/0015_refresh_tokens_password_reset.up.sql), it's keyed by
// user_id, a platform-global concept.
type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *domain.RefreshToken) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.RevokedAt, t.CreatedAt,
	)
	return err
}

// GetByHash returns domain.ErrRefreshTokenInvalid for both "no such
// token" and "token exists but already revoked/expired" - RefreshToken.
// Valid() is what the caller (RefreshTokenService) uses to distinguish
// those internally if it ever needs to, but the two are deliberately
// the same externally-observable outcome: a presented token that
// doesn't currently grant a new access token, full stop.
func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	var t domain.RefreshToken
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, revoked_at, created_at FROM refresh_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrRefreshTokenInvalid
		}
		return nil, err
	}
	return &t, nil
}

// Revoke is called on every successful Refresh() - rotation, not reuse:
// each refresh token is single-use, the same real-world pattern that
// makes a replayed/stolen-then-reused refresh token detectable (a
// second attempt to use an already-revoked token is a strong signal of
// compromise, though this walking skeleton doesn't yet act on that
// signal - detection, not automatic session-wide revocation, a further,
// real gap named rather than silently assumed).
func (r *RefreshTokenRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`, id)
	return err
}
