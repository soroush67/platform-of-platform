package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	"platform-of-platform/internal/workspace/adapters/postgres"
	"platform-of-platform/internal/workspace/domain"
)

func TestEnvironmentRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewEnvironmentRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	env, err := domain.NewEnvironment(orgID, projectID, "production", 2, true)
	if err != nil {
		t.Fatalf("NewEnvironment: %v", err)
	}
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM environments WHERE id = $1`, env.ID) })

	got, err := repo.GetByID(ctx, orgID, env.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "production" || got.PromotionRank != 2 || !got.RequiresApproval {
		t.Errorf("expected fields to round-trip, got %+v", got)
	}
}

func TestEnvironmentRepository_GetByID_WrongOrganizationReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewEnvironmentRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	env, _ := domain.NewEnvironment(orgID, projectID, "staging", 1, false)
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM environments WHERE id = $1`, env.ID) })

	_, err := repo.GetByID(ctx, uuid.NewString(), env.ID)
	if !errors.Is(err, domain.ErrEnvironmentNotFound) {
		t.Fatalf("expected ErrEnvironmentNotFound for an environment under a different org, got: %v", err)
	}
}

func TestEnvironmentRepository_Exists(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewEnvironmentRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	exists, err := repo.Exists(ctx, orgID, uuid.NewString())
	if err != nil {
		t.Fatalf("Exists (unknown): %v", err)
	}
	if exists {
		t.Error("expected an unknown environment id to not exist")
	}

	env, _ := domain.NewEnvironment(orgID, projectID, "dev", 0, false)
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM environments WHERE id = $1`, env.ID) })

	exists, err = repo.Exists(ctx, orgID, env.ID)
	if err != nil {
		t.Fatalf("Exists (real): %v", err)
	}
	if !exists {
		t.Error("expected a real environment to exist")
	}
}

func TestEnvironmentRepository_ListByProject_OrderedByPromotionRank(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewEnvironmentRepository(pool)
	orgID, projectID := insertOrgAndProject(t, root)

	prod, _ := domain.NewEnvironment(orgID, projectID, "production", 2, true)
	staging, _ := domain.NewEnvironment(orgID, projectID, "staging", 1, false)
	dev, _ := domain.NewEnvironment(orgID, projectID, "dev", 0, false)
	// Deliberately created out of promotion-rank order - the query's own
	// ORDER BY promotion_rank is what this test actually verifies.
	for _, e := range []*domain.Environment{prod, staging, dev} {
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create %s: %v", e.Name, err)
		}
		t.Cleanup(func(id string) func() {
			return func() { mustExec(t, root, `DELETE FROM environments WHERE id = $1`, id) }
		}(e.ID))
	}

	got, err := repo.ListByProject(ctx, orgID, projectID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 environments, got %d", len(got))
	}
	if got[0].Name != "dev" || got[1].Name != "staging" || got[2].Name != "production" {
		t.Errorf("expected environments ordered by promotion_rank (dev, staging, production), got %s, %s, %s", got[0].Name, got[1].Name, got[2].Name)
	}
}
