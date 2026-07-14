package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	// GetByUsername returns domain.ErrUserNotFound if no such user exists.
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
}
