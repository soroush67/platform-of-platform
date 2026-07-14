package application

import (
	"context"
	"errors"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
)

// PasswordResetService - real token generation, real hashed storage,
// real expiry/single-use enforcement. The one piece deliberately NOT
// built here: actual email delivery - this codebase has no SMTP/email
// integration at all (a separate, already-named gap). RequestReset logs
// the plaintext token server-side via logger.Info instead of emailing
// it, clearly flagged as a stand-in a real deployment must replace with
// a real mail sender, not silently pretended to be a finished feature.
type PasswordResetService struct {
	tokenRepo PasswordResetTokenRepository
	userRepo  UserRepository
	logger    *slog.Logger
}

func NewPasswordResetService(tokenRepo PasswordResetTokenRepository, userRepo UserRepository, logger *slog.Logger) *PasswordResetService {
	return &PasswordResetService{tokenRepo: tokenRepo, userRepo: userRepo, logger: logger}
}

// RequestReset never returns an error for "no such email" or "this
// account has no local password to reset" - same "don't reveal
// existence" posture as every lookup-by-identifier path in this
// codebase (docs/architecture's own established rule, first applied to
// Organization lookups). The HTTP handler always returns 202 regardless
// of what happened here.
func (s *PasswordResetService) RequestReset(ctx context.Context, email string) error {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil
		}
		return err
	}
	if user.AuthSource != domain.AuthSourceLocal {
		// An SSO user has no local password to reset - same non-revealing
		// no-op, not a distinguishable error.
		return nil
	}

	plaintext, err := auth.GenerateOpaqueToken()
	if err != nil {
		return err
	}

	token := domain.NewPasswordResetToken(user.ID, auth.HashOpaqueToken(plaintext))
	if err := s.tokenRepo.Create(ctx, token); err != nil {
		return err
	}

	// Stand-in for real email delivery (no SMTP integration exists in
	// this codebase) - logged, not returned in the HTTP response; an
	// HTTP response is something any caller of this endpoint can read,
	// which would defeat the entire point of sending it out-of-band to
	// a verified email address instead.
	s.logger.Info("password reset requested - no email integration, logging token instead",
		"user_id", user.ID, "username", user.Username, "reset_token", plaintext)

	return nil
}

// ConfirmReset validates the presented token, hashes and sets the new
// password, marks the token used (single-use, same posture as
// RefreshTokenService's rotation).
func (s *PasswordResetService) ConfirmReset(ctx context.Context, plaintextToken, newPassword string) error {
	if len(newPassword) < 8 {
		return &domain.ValidationError{Message: "password must be at least 8 characters"}
	}

	hash := auth.HashOpaqueToken(plaintextToken)
	token, err := s.tokenRepo.GetByHash(ctx, hash)
	if err != nil {
		return err
	}
	if !token.Valid() {
		return domain.ErrPasswordResetTokenInvalid
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePasswordHash(ctx, token.UserID, string(hashedPassword)); err != nil {
		return err
	}

	return s.tokenRepo.MarkUsed(ctx, token.ID)
}
