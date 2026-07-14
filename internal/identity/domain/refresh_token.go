package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRefreshTokenInvalid = errors.New("refresh token is invalid, expired, or already used")
)

// RefreshTokenTTL - real, but deliberately much longer than
// auth.AccessTokenTTL (15m): the access token stays short-lived (no
// revocation list, self-invalidates fast if stolen); the refresh token
// is the thing a client actually holds onto across a session, revoked
// explicitly (rotation on use) rather than relying on a short TTL alone.
const RefreshTokenTTL = 30 * 24 * time.Hour

// RefreshToken is a real, hashed, single-use (rotated on every
// Refresh() call - see RefreshTokenService) bearer credential - never
// held as plaintext past the moment it's issued (TokenHash is a SHA-256
// digest, internal/platform/auth.HashOpaqueToken).
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func NewRefreshToken(userID, tokenHash string) *RefreshToken {
	now := time.Now().UTC()
	return &RefreshToken{
		ID:        uuid.NewString(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(RefreshTokenTTL),
		CreatedAt: now,
	}
}

// Valid is the same check RefreshAccessToken applies at lookup time -
// factored out so a real, testable domain method exists instead of the
// application layer inlining the same two comparisons.
func (t *RefreshToken) Valid() bool {
	return t.RevokedAt == nil && time.Now().UTC().Before(t.ExpiresAt)
}
