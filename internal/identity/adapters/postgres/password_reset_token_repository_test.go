package postgres_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/identity/adapters/postgres"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/dbtest"
)

func TestPasswordResetTokenRepository_CreateAndGetByHash(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	userRepo := postgres.NewUserRepository(pool)
	repo := postgres.NewPasswordResetTokenRepository(pool)

	u := mustLocalUser(t)
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	rt := domain.NewPasswordResetToken(u.ID, "a-reset-token-hash")
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM password_reset_tokens WHERE id = $1`, rt.ID) })
	if err := repo.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-reset-token-hash")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.ID != rt.ID || got.UserID != u.ID {
		t.Errorf("expected the created token to round-trip, got %+v", got)
	}
	if !got.Valid() {
		t.Error("expected a freshly created token to be valid")
	}
}

func TestPasswordResetTokenRepository_GetByHash_UnknownReturnsInvalid(t *testing.T) {
	pool := dbtest.AppPool(t)
	repo := postgres.NewPasswordResetTokenRepository(pool)

	_, err := repo.GetByHash(context.Background(), "no-such-reset-hash-ever")
	if !errors.Is(err, domain.ErrPasswordResetTokenInvalid) {
		t.Fatalf("expected ErrPasswordResetTokenInvalid, got: %v", err)
	}
}

func TestPasswordResetTokenRepository_MarkUsed(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	userRepo := postgres.NewUserRepository(pool)
	repo := postgres.NewPasswordResetTokenRepository(pool)

	u := mustLocalUser(t)
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	rt := domain.NewPasswordResetToken(u.ID, "a-markused-test-hash")
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM password_reset_tokens WHERE id = $1`, rt.ID) })
	if err := repo.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.MarkUsed(ctx, rt.ID); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-markused-test-hash")
	if err != nil {
		t.Fatalf("GetByHash after MarkUsed: %v", err)
	}
	if got.UsedAt == nil {
		t.Error("expected UsedAt to be set after MarkUsed")
	}
	if got.Valid() {
		t.Error("expected a used token to be invalid")
	}
}
