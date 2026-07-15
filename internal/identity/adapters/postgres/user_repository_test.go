package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/identity/adapters/postgres"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/dbtest"
)

func mustLocalUser(t *testing.T) *domain.User {
	t.Helper()
	suffix := uuid.NewString()[:8]
	u, err := domain.NewUser("adapter-user-"+suffix, "adapter-user-"+suffix+"@example.com", domain.AuthSourceLocal)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if err := u.SetPasswordHash("$2a$10$fakebcryptfakebcryptfakebcryptfakebcryptfakebcrypt"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	return u
}

func TestUserRepository_CreateAndGetByUsername(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewUserRepository(pool)

	u := mustLocalUser(t)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	got, err := repo.GetByUsername(ctx, u.Username)
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.ID != u.ID || got.Email != u.Email {
		t.Errorf("expected the created user to round-trip, got %+v", got)
	}
	if got.PasswordHash == nil || *got.PasswordHash != *u.PasswordHash {
		t.Error("expected PasswordHash to round-trip")
	}
}

func TestUserRepository_GetByUsername_UnknownReturnsNotFound(t *testing.T) {
	pool := dbtest.AppPool(t)
	repo := postgres.NewUserRepository(pool)

	_, err := repo.GetByUsername(context.Background(), "no-such-user-ever")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestUserRepository_GetByIDAndGetByEmail(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewUserRepository(pool)

	u := mustLocalUser(t)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	byID, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Username != u.Username {
		t.Errorf("expected username %q, got %q", u.Username, byID.Username)
	}

	byEmail, err := repo.GetByEmail(ctx, u.Email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != u.ID {
		t.Errorf("expected GetByEmail to find the same user, got id %q", byEmail.ID)
	}
}

func TestUserRepository_UpdatePasswordHash(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewUserRepository(pool)

	u := mustLocalUser(t)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	newHash := "$2a$10$anewfakebcryptfakebcryptfakebcryptfakebcryptfakebcr"
	if err := repo.UpdatePasswordHash(ctx, u.ID, newHash); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PasswordHash == nil || *got.PasswordHash != newHash {
		t.Errorf("expected the updated hash to persist, got %v", got.PasswordHash)
	}
}
