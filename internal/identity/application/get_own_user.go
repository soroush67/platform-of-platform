package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

// GetOwnUserService implements `GET /api/v1/users/me` - "look up
// yourself," no permission check needed since the id being looked up is
// the caller's own (httpserver.UserIDFromContext), the same bare
// GetByID passthrough RefreshTokenService.Refresh already uses for its
// own defensive existence check.
type GetOwnUserService struct {
	userRepo UserRepository
}

func NewGetOwnUserService(userRepo UserRepository) *GetOwnUserService {
	return &GetOwnUserService{userRepo: userRepo}
}

func (s *GetOwnUserService) Execute(ctx context.Context, userID string) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}
