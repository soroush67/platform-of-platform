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

func TestAPIKeyRepository_CreateAndGetByHash(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAPIKeyRepository(pool)
	orgID := insertOrg(t, root)

	key, err := domain.NewAPIKey(domain.APIKeyOwnerTypeServiceAccount, uuid.NewString(), "ci-key", "a-real-key-hash", []string{"workspace:read"}, nil)
	if err != nil {
		t.Fatalf("NewAPIKey: %v", err)
	}
	if err := repo.Create(ctx, orgID, key); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM api_keys WHERE id = $1`, key.ID) })

	got, err := repo.GetByHash(ctx, "a-real-key-hash")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.ID != key.ID || got.Name != "ci-key" {
		t.Errorf("expected fields to round-trip, got %+v", got)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "workspace:read" {
		t.Errorf("expected scopes to round-trip through the jsonb column, got %v", got.Scopes)
	}
}

func TestAPIKeyRepository_GetByHash_UnknownReturnsInvalid(t *testing.T) {
	pool := dbtest.AppPool(t)
	repo := postgres.NewAPIKeyRepository(pool)

	_, err := repo.GetByHash(context.Background(), "no-such-key-hash-ever")
	if !errors.Is(err, domain.ErrAPIKeyInvalid) {
		t.Fatalf("expected ErrAPIKeyInvalid, got: %v", err)
	}
}

func TestAPIKeyRepository_TouchLastUsed(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAPIKeyRepository(pool)
	orgID := insertOrg(t, root)

	key, _ := domain.NewAPIKey(domain.APIKeyOwnerTypeUser, uuid.NewString(), "touch-key", "a-touch-key-hash", nil, nil)
	if err := repo.Create(ctx, orgID, key); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM api_keys WHERE id = $1`, key.ID) })

	if err := repo.TouchLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-touch-key-hash")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set after TouchLastUsed")
	}
}

func TestAPIKeyRepository_Revoke(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAPIKeyRepository(pool)
	orgID := insertOrg(t, root)

	key, _ := domain.NewAPIKey(domain.APIKeyOwnerTypeUser, uuid.NewString(), "revoke-key", "a-revoke-key-hash", nil, nil)
	if err := repo.Create(ctx, orgID, key); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM api_keys WHERE id = $1`, key.ID) })

	if err := repo.Revoke(ctx, orgID, key.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-revoke-key-hash")
	if err != nil {
		t.Fatalf("GetByHash after revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("expected RevokedAt to be set after Revoke")
	}
	if got.Valid() {
		t.Error("expected a revoked key to be invalid")
	}

	// A second Revoke on an already-revoked key must fail, not silently
	// succeed - Revoke's own WHERE revoked_at IS NULL guard.
	if err := repo.Revoke(ctx, orgID, key.ID); !errors.Is(err, domain.ErrAPIKeyInvalid) {
		t.Fatalf("expected ErrAPIKeyInvalid revoking an already-revoked key, got: %v", err)
	}
}

func TestAPIKeyRepository_Revoke_WrongOrganizationRejected(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAPIKeyRepository(pool)
	orgID := insertOrg(t, root)

	key, _ := domain.NewAPIKey(domain.APIKeyOwnerTypeUser, uuid.NewString(), "cross-org-key", "a-cross-org-key-hash", nil, nil)
	if err := repo.Create(ctx, orgID, key); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM api_keys WHERE id = $1`, key.ID) })

	// api_keys has no RLS at all (deliberately - see the type's own doc
	// comment) - Revoke's explicit "AND organization_id = $2" in its own
	// WHERE clause is the *only* thing enforcing this boundary, so this
	// is a real, meaningful regression test, not belt-and-braces on top
	// of RLS the way the RLS-backed tables' tests are.
	err := repo.Revoke(ctx, uuid.NewString(), key.ID)
	if !errors.Is(err, domain.ErrAPIKeyInvalid) {
		t.Fatalf("expected ErrAPIKeyInvalid revoking a real key under the wrong organization_id, got: %v", err)
	}

	got, err := repo.GetByHash(ctx, "a-cross-org-key-hash")
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if got.RevokedAt != nil {
		t.Error("expected the key to remain un-revoked after a wrong-org Revoke attempt")
	}
}
