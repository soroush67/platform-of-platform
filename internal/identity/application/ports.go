package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	// GetByUsername returns domain.ErrUserNotFound if no such user exists.
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
}

// RefreshTokenRepository is RefreshTokenService's own port - previously
// entirely unbuilt (access tokens had no companion refresh mechanism at
// all).
type RefreshTokenRepository interface {
	Create(ctx context.Context, t *domain.RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Revoke(ctx context.Context, id string) error
}

// PasswordResetTokenRepository is PasswordResetService's own port.
type PasswordResetTokenRepository interface {
	Create(ctx context.Context, t *domain.PasswordResetToken) error
	GetByHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error)
	MarkUsed(ctx context.Context, id string) error
}
