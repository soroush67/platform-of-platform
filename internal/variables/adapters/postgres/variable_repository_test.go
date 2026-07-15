package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	"platform-of-platform/internal/variables/adapters/postgres"
	"platform-of-platform/internal/variables/domain"
)

func TestVariableRepository_CreateAndGetByScope(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	v, err := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "bar")
	if err != nil {
		t.Fatalf("NewVariable: %v", err)
	}
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByScope(ctx, orgID, domain.ScopeTypeOrganization, orgID, "FOO")
	if err != nil {
		t.Fatalf("GetByScope: %v", err)
	}
	if got.Value != "bar" || got.Category != domain.CategoryEnvVar {
		t.Errorf("expected fields to round-trip, got %+v", got)
	}
}

func TestVariableRepository_GetByScope_UnknownReturnsNotFound(t *testing.T) {
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	_, err := repo.GetByScope(context.Background(), orgID, domain.ScopeTypeOrganization, orgID, "NOPE")
	if !errors.Is(err, domain.ErrVariableNotFound) {
		t.Fatalf("expected ErrVariableNotFound, got: %v", err)
	}
}

func TestVariableRepository_GetByScope_DifferentScopeSameKeyIsIndependent(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	workspaceID := uuid.NewString()
	orgVar, _ := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "org-value")
	wsVar, _ := domain.NewVariable(orgID, domain.ScopeTypeWorkspace, workspaceID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "ws-value")
	if err := repo.Create(ctx, orgVar); err != nil {
		t.Fatalf("Create orgVar: %v", err)
	}
	if err := repo.Create(ctx, wsVar); err != nil {
		t.Fatalf("Create wsVar: %v", err)
	}

	got, err := repo.GetByScope(ctx, orgID, domain.ScopeTypeWorkspace, workspaceID, "FOO")
	if err != nil {
		t.Fatalf("GetByScope (workspace): %v", err)
	}
	if got.Value != "ws-value" {
		t.Errorf("expected the workspace-scoped FOO, got %q", got.Value)
	}
}

func TestVariableRepository_ListByScope(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	v1, _ := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "1")
	v2, _ := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "BAR", domain.CategoryEnvVar, domain.SensitivityPlain, "2")
	otherScope, _ := domain.NewVariable(orgID, domain.ScopeTypeWorkspace, uuid.NewString(), "BAZ", domain.CategoryEnvVar, domain.SensitivityPlain, "3")
	for _, v := range []*domain.Variable{v1, v2, otherScope} {
		if err := repo.Create(ctx, v); err != nil {
			t.Fatalf("Create %s: %v", v.Key, err)
		}
	}

	got, err := repo.ListByScope(ctx, orgID, domain.ScopeTypeOrganization, orgID)
	if err != nil {
		t.Fatalf("ListByScope: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly the 2 organization-scoped variables, got %d", len(got))
	}
	if got[0].Key != "BAR" || got[1].Key != "FOO" {
		t.Errorf("expected alphabetical key ordering [BAR, FOO], got [%s, %s]", got[0].Key, got[1].Key)
	}
}

func TestVariableRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	v, _ := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "bar")
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, v.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Key != "FOO" {
		t.Errorf("expected key %q, got %q", "FOO", got.Key)
	}

	_, err = repo.GetByID(ctx, uuid.NewString(), v.ID)
	if !errors.Is(err, domain.ErrVariableNotFound) {
		t.Fatalf("expected ErrVariableNotFound for the wrong org, got: %v", err)
	}
}

func TestVariableRepository_Update_LeavesKeyAndScopeImmutable(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	v, _ := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "original")
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("Create: %v", err)
	}

	v.Value = "updated"
	v.Category = domain.CategoryEngineVar
	v.Sensitivity = domain.SensitivitySensitive
	if err := repo.Update(ctx, v); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, v.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Value != "updated" || got.Category != domain.CategoryEngineVar || got.Sensitivity != domain.SensitivitySensitive {
		t.Errorf("expected the update to apply, got %+v", got)
	}
	if got.Key != "FOO" || got.ScopeType != domain.ScopeTypeOrganization {
		t.Errorf("expected key/scope to stay immutable, got key=%q scope=%q", got.Key, got.ScopeType)
	}
}

func TestVariableRepository_Delete(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewVariableRepository(pool)
	orgID := insertOrg(t, root)

	v, _ := domain.NewVariable(orgID, domain.ScopeTypeOrganization, orgID, "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "bar")
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, orgID, v.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(ctx, orgID, v.ID)
	if !errors.Is(err, domain.ErrVariableNotFound) {
		t.Fatalf("expected ErrVariableNotFound after Delete, got: %v", err)
	}
}
