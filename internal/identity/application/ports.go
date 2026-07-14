package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
}
