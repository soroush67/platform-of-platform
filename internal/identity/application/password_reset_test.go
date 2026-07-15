package application_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

func TestPasswordResetService_RequestResetIsSilentForUnknownEmail(t *testing.T) {
	var logs bytes.Buffer
	svc := application.NewPasswordResetService(newFakePasswordResetTokenRepo(), newFakeUserRepo(), slog.New(slog.NewTextHandler(&logs, nil)))

	// No such user - RequestReset must not error (the "don't reveal
	// existence" invariant its own doc comment names) and must not log
	// a token that was never generated.
	if err := svc.RequestReset(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("expected no error for an unknown email, got: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("expected no token to be logged for an unknown email, got: %s", logs.String())
	}
}

func TestPasswordResetService_RequestResetIsSilentForSSOUser(t *testing.T) {
	userRepo := newFakeUserRepo()
	ssoUser, _ := domain.NewUser("carol", "carol@example.com", domain.AuthSourceOIDC)
	userRepo.put(ssoUser)
	var logs bytes.Buffer
	svc := application.NewPasswordResetService(newFakePasswordResetTokenRepo(), userRepo, slog.New(slog.NewTextHandler(&logs, nil)))

	if err := svc.RequestReset(context.Background(), "carol@example.com"); err != nil {
		t.Fatalf("expected no error for an SSO user, got: %v", err)
	}
	if logs.Len() != 0 {
		t.Error("expected no reset token to be generated for a user with no local password")
	}
}

func TestPasswordResetService_FullRequestConfirmCycle(t *testing.T) {
	userRepo := newFakeUserRepo()
	user := mustLocalUser(t, "alice", "old-password")
	userRepo.put(user)
	var logs bytes.Buffer
	tokenRepo := newFakePasswordResetTokenRepo()
	svc := application.NewPasswordResetService(tokenRepo, userRepo, slog.New(slog.NewTextHandler(&logs, nil)))

	if err := svc.RequestReset(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("RequestReset: %v", err)
	}

	// Pull the real token back out of the log line - the deliberate
	// SMTP stand-in this service's own doc comment names.
	logOutput := logs.String()
	const marker = "reset_token="
	idx := strings.Index(logOutput, marker)
	if idx == -1 {
		t.Fatalf("expected a reset_token in the log output, got: %s", logOutput)
	}
	rest := logOutput[idx+len(marker):]
	token := strings.Fields(rest)[0]

	if err := svc.ConfirmReset(context.Background(), token, "new-password-123"); err != nil {
		t.Fatalf("ConfirmReset: %v", err)
	}

	updated, _ := userRepo.GetByID(context.Background(), user.ID)
	if err := bcrypt.CompareHashAndPassword([]byte(*updated.PasswordHash), []byte("new-password-123")); err != nil {
		t.Errorf("expected the new password to verify, got: %v", err)
	}

	// Single-use: reusing the same token a second time must fail.
	if err := svc.ConfirmReset(context.Background(), token, "yet-another-password"); !errors.Is(err, domain.ErrPasswordResetTokenInvalid) {
		t.Errorf("expected a reused token to be rejected, got: %v", err)
	}
}

func TestPasswordResetService_ConfirmRejectsShortPassword(t *testing.T) {
	svc := application.NewPasswordResetService(newFakePasswordResetTokenRepo(), newFakeUserRepo(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

	err := svc.ConfirmReset(context.Background(), "some-token", "short")
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for a too-short password, got: %v", err)
	}
}
