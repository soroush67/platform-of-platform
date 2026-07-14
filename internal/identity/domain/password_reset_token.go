package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrPasswordResetTokenInvalid = errors.New("password reset token is invalid, expired, or already used")

// PasswordResetTokenTTL - short and single-use on purpose (a reset
// token that grants "set this account's password to anything" is a
// materially higher-consequence credential than a refresh token; a
// real deployment sending this via email expects the recipient to act
// within minutes, not weeks).
const PasswordResetTokenTTL = 30 * time.Minute

type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func NewPasswordResetToken(userID, tokenHash string) *PasswordResetToken {
	now := time.Now().UTC()
	return &PasswordResetToken{
		ID:        uuid.NewString(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(PasswordResetTokenTTL),
		CreatedAt: now,
	}
}

func (t *PasswordResetToken) Valid() bool {
	return t.UsedAt == nil && time.Now().UTC().Before(t.ExpiresAt)
}
