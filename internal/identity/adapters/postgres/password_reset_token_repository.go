package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/identity/domain"
)

type PasswordResetTokenRepository struct {
	pool *pgxpool.Pool
}

func NewPasswordResetTokenRepository(pool *pgxpool.Pool) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{pool: pool}
}

func (r *PasswordResetTokenRepository) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.UsedAt, t.CreatedAt,
	)
	return err
}

func (r *PasswordResetTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	var t domain.PasswordResetToken
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at, created_at FROM password_reset_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrPasswordResetTokenInvalid
		}
		return nil, err
	}
	return &t, nil
}

func (r *PasswordResetTokenRepository) MarkUsed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE id = $1`, id)
	return err
}
