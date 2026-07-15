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

func TestServiceAccountRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewServiceAccountRepository(pool)
	orgID := insertOrg(t, root)

	sa, err := domain.NewServiceAccount(orgID, "deploy-bot", "used by CI")
	if err != nil {
		t.Fatalf("NewServiceAccount: %v", err)
	}
	if err := repo.Create(ctx, sa); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM service_accounts WHERE id = $1`, sa.ID) })

	got, err := repo.GetByID(ctx, orgID, sa.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "deploy-bot" || got.Description != "used by CI" {
		t.Errorf("expected fields to round-trip, got %+v", got)
	}
}

func TestServiceAccountRepository_GetByID_WrongOrganizationReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewServiceAccountRepository(pool)
	orgID := insertOrg(t, root)

	sa, _ := domain.NewServiceAccount(orgID, "deploy-bot", "")
	if err := repo.Create(ctx, sa); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM service_accounts WHERE id = $1`, sa.ID) })

	_, err := repo.GetByID(ctx, uuid.NewString(), sa.ID)
	if !errors.Is(err, domain.ErrServiceAccountNotFound) {
		t.Fatalf("expected ErrServiceAccountNotFound for a service account under a different org, got: %v", err)
	}
}

func TestServiceAccountRepository_ServiceAccountExists(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewServiceAccountRepository(pool)
	orgID := insertOrg(t, root)

	exists, err := repo.ServiceAccountExists(ctx, orgID, uuid.NewString())
	if err != nil {
		t.Fatalf("ServiceAccountExists (unknown): %v", err)
	}
	if exists {
		t.Error("expected an unknown service account id to not exist")
	}

	sa, _ := domain.NewServiceAccount(orgID, "deploy-bot", "")
	if err := repo.Create(ctx, sa); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM service_accounts WHERE id = $1`, sa.ID) })

	exists, err = repo.ServiceAccountExists(ctx, orgID, sa.ID)
	if err != nil {
		t.Fatalf("ServiceAccountExists (real): %v", err)
	}
	if !exists {
		t.Error("expected a real service account to exist")
	}
}
