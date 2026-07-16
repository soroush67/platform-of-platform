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

// TestUserRepository_GetUser proves GetUser (the primitives-only sibling
// of GetByID that satisfies Tenancy's own UserReader port for the
// member roster) round-trips real data and reports found=false, not an
// error, for an unknown id.
func TestUserRepository_GetUser(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewUserRepository(pool)

	u := mustLocalUser(t)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	username, email, found, err := repo.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !found || username != u.Username || email != u.Email {
		t.Errorf("expected found=true with username=%q email=%q, got found=%v username=%q email=%q", u.Username, u.Email, found, username, email)
	}

	_, _, found, err = repo.GetUser(ctx, uuid.NewString())
	if err != nil {
		t.Fatalf("GetUser (unknown id): %v", err)
	}
	if found {
		t.Error("expected found=false for an unknown user id, not an error")
	}
}

// TestUserRepository_Count_ReflectsRealInsert asserts a delta, not an
// absolute count - the users table is shared with every other test in
// this package (and leftover rows from real usage), so an absolute
// assertion would be flaky by construction. Proving Count actually
// tracks a real INSERT is the honest, real thing to check.
func TestUserRepository_Count_ReflectsRealInsert(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewUserRepository(pool)

	before, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count (before): %v", err)
	}

	u := mustLocalUser(t)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	after, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count (after): %v", err)
	}
	if after != before+1 {
		t.Errorf("expected Count to increase by exactly 1 after a real Create, got before=%d after=%d", before, after)
	}
}

func TestUserRepository_IsPlatformAdminAndSetPlatformAdmin(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewUserRepository(pool)

	u := mustLocalUser(t)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM users WHERE id = $1`, u.ID) })

	isAdmin, err := repo.IsPlatformAdmin(ctx, u.ID)
	if err != nil {
		t.Fatalf("IsPlatformAdmin (before): %v", err)
	}
	if isAdmin {
		t.Error("expected a freshly-created user to default to is_platform_admin=false")
	}

	if err := repo.SetPlatformAdmin(ctx, u.ID, true); err != nil {
		t.Fatalf("SetPlatformAdmin: %v", err)
	}

	isAdmin, err = repo.IsPlatformAdmin(ctx, u.ID)
	if err != nil {
		t.Fatalf("IsPlatformAdmin (after): %v", err)
	}
	if !isAdmin {
		t.Error("expected is_platform_admin=true after SetPlatformAdmin(true)")
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
