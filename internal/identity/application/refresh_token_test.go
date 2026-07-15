package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

func TestRefreshTokenService_IssueThenRefreshRotatesTheToken(t *testing.T) {
	tokenRepo := newFakeRefreshTokenRepo()
	userRepo := newFakeUserRepo()
	user, _ := domain.NewUser("alice", "alice@example.com", domain.AuthSourceLocal)
	userRepo.put(user)
	svc := application.NewRefreshTokenService(tokenRepo, userRepo)

	plaintext, err := svc.Issue(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected a real, non-empty plaintext token")
	}

	gotUserID, newToken, err := svc.Refresh(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if gotUserID != user.ID {
		t.Errorf("expected refresh to resolve to user %q, got %q", user.ID, gotUserID)
	}
	if newToken == plaintext {
		t.Error("expected a genuinely new token, not the same one echoed back")
	}

	// Single-use rotation: the OLD token must no longer work.
	if _, _, err := svc.Refresh(context.Background(), plaintext); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Errorf("expected the rotated-away old token to be rejected, got: %v", err)
	}

	// The NEW token must still work.
	if _, _, err := svc.Refresh(context.Background(), newToken); err != nil {
		t.Errorf("expected the new token to still be valid, got: %v", err)
	}
}

func TestRefreshTokenService_UnknownTokenRejected(t *testing.T) {
	svc := application.NewRefreshTokenService(newFakeRefreshTokenRepo(), newFakeUserRepo())

	_, _, err := svc.Refresh(context.Background(), "this-token-was-never-issued")
	if !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("expected ErrRefreshTokenInvalid, got: %v", err)
	}
}
