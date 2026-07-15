package postgres_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/identity/adapters/postgres"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/dbtest"
)

func TestRefreshTokenRepository_CreateAndGetByHash(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	userRepo := postgres.NewUserRepository(pool)
	repo := postgres.NewRefreshTokenRepository(pool)

	u := mustLocalUser(t)
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	rt := domain.NewRefreshToken(u.ID, "a-real-token-hash")
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM refresh_tokens WHERE id = $1`, rt.ID) })
	if err := repo.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-real-token-hash")
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

func TestRefreshTokenRepository_GetByHash_UnknownReturnsInvalid(t *testing.T) {
	pool := dbtest.AppPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)

	_, err := repo.GetByHash(context.Background(), "no-such-hash-ever")
	if !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("expected ErrRefreshTokenInvalid, got: %v", err)
	}
}

func TestRefreshTokenRepository_Revoke(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	userRepo := postgres.NewUserRepository(pool)
	repo := postgres.NewRefreshTokenRepository(pool)

	u := mustLocalUser(t)
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("Create user: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	rt := domain.NewRefreshToken(u.ID, "a-revoke-test-hash")
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM refresh_tokens WHERE id = $1`, rt.ID) })
	if err := repo.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Revoke(ctx, rt.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-revoke-test-hash")
	if err != nil {
		t.Fatalf("GetByHash after revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("expected RevokedAt to be set after Revoke")
	}
	if got.Valid() {
		t.Error("expected a revoked token to be invalid")
	}
}
