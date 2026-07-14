package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
)

// RefreshTokenService closes the "access token is a hard 15-minute TTL,
// then you have to log in again" gap - Issue is called once, right
// after a successful login (LoginHandler); Refresh is
// `POST /auth/refresh`'s own use case, rotating the presented token
// (single-use) and returning a new pair.
type RefreshTokenService struct {
	repo     RefreshTokenRepository
	userRepo UserRepository
}

func NewRefreshTokenService(repo RefreshTokenRepository, userRepo UserRepository) *RefreshTokenService {
	return &RefreshTokenService{repo: repo, userRepo: userRepo}
}

// Issue mints a real refresh token for userID - called from
// LoginHandler right after AuthenticateService succeeds, not a separate
// authenticated endpoint (there's nothing to authenticate with yet at
// that point beyond the login credentials themselves).
func (s *RefreshTokenService) Issue(ctx context.Context, userID string) (plaintext string, err error) {
	plaintext, err = auth.GenerateOpaqueToken()
	if err != nil {
		return "", err
	}

	token := domain.NewRefreshToken(userID, auth.HashOpaqueToken(plaintext))
	if err := s.repo.Create(ctx, token); err != nil {
		return "", err
	}

	return plaintext, nil
}

// Refresh validates the presented plaintext token, revokes it (rotation:
// each refresh token is single-use), and issues a new one - returning
// the user id (so the HTTP handler can mint a fresh access token the
// same way LoginHandler does) and the new refresh token's plaintext.
func (s *RefreshTokenService) Refresh(ctx context.Context, plaintextToken string) (userID, newRefreshToken string, err error) {
	hash := auth.HashOpaqueToken(plaintextToken)

	token, err := s.repo.GetByHash(ctx, hash)
	if err != nil {
		return "", "", err
	}
	if !token.Valid() {
		return "", "", domain.ErrRefreshTokenInvalid
	}

	// Confirm the user this token belongs to still exists and is real -
	// a deleted/suspended-account edge case this codebase doesn't yet
	// have a Suspend flow for, but the lookup itself is still the right
	// defensive check (GetByID's own domain.ErrUserNotFound propagates
	// as a real error here, not silently issuing a token pair for a
	// user_id that no longer resolves to anything).
	if _, err := s.userRepo.GetByID(ctx, token.UserID); err != nil {
		return "", "", err
	}

	if err := s.repo.Revoke(ctx, token.ID); err != nil {
		return "", "", err
	}

	newRefreshToken, err = s.Issue(ctx, token.UserID)
	if err != nil {
		return "", "", err
	}

	return token.UserID, newRefreshToken, nil
}
