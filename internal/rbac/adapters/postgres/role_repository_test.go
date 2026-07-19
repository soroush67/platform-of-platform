package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	rbacpg "platform-of-platform/internal/rbac/adapters/postgres"
	"platform-of-platform/internal/rbac/domain"
)

func TestRoleRepository_SeedBuiltinRoles_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	repo := rbacpg.NewRoleRepository(pool)

	if err := repo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles (first): %v", err)
	}
	if err := repo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles (second): %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM roles WHERE name = $1 AND organization_id IS NULL`, domain.RoleOwner).Scan(&count); err != nil {
		t.Fatalf("query roles: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 built-in %q role row despite seeding twice, got %d", domain.RoleOwner, count)
	}
}

func TestRoleRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := rbacpg.NewRoleRepository(pool)
	orgID := insertOrg(t, root)

	role, err := domain.NewRole(orgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply, domain.PermissionWorkspaceRead})
	if err != nil {
		t.Fatalf("NewRole: %v", err)
	}
	if err := repo.Create(ctx, role); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	got, err := repo.GetByID(ctx, orgID, role.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "deployer" || len(got.Permissions) != 2 {
		t.Errorf("expected the role to round-trip, got %+v", got)
	}
}

func TestRoleRepository_Update_RewritesPermissionsInPlace(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := rbacpg.NewRoleRepository(pool)
	orgID := insertOrg(t, root)

	role, _ := domain.NewRole(orgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	if err := repo.Create(ctx, role); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	role.Permissions = []domain.Permission{domain.PermissionWorkspaceRead, domain.PermissionWorkspaceManage}
	if err := repo.Update(ctx, role); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, orgID, role.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "deployer" {
		t.Errorf("expected the name to stay unchanged, got %q", got.Name)
	}
	if len(got.Permissions) != 2 {
		t.Errorf("expected the updated 2-permission set, got %+v", got.Permissions)
	}
}

func TestRoleRepository_Update_BuiltinRoleRejected(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	repo := rbacpg.NewRoleRepository(pool)

	if err := repo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles: %v", err)
	}
	builtin := &domain.Role{ID: uuid.NewString(), OrganizationID: nil, Name: domain.RoleOwner, Permissions: []domain.Permission{domain.PermissionOrganizationManage}}

	var validationErr *domain.ValidationError
	if err := repo.Update(ctx, builtin); !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError attempting to update a built-in role, got: %v", err)
	}
}

func TestRoleRepository_Create_DuplicateNameRejected(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := rbacpg.NewRoleRepository(pool)
	orgID := insertOrg(t, root)

	role1, _ := domain.NewRole(orgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	if err := repo.Create(ctx, role1); err != nil {
		t.Fatalf("Create (first): %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role1.ID) })

	role2, _ := domain.NewRole(orgID, "deployer", []domain.Permission{domain.PermissionWorkspaceRead})
	err := repo.Create(ctx, role2)
	if !errors.Is(err, domain.ErrRoleAlreadyExists) {
		t.Fatalf("expected ErrRoleAlreadyExists for a duplicate name in the same org, got: %v", err)
	}
}

func TestRoleRepository_GetByID_UnknownReturnsNotFound(t *testing.T) {
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := rbacpg.NewRoleRepository(pool)
	orgID := insertOrg(t, root)

	_, err := repo.GetByID(context.Background(), orgID, uuid.NewString())
	if !errors.Is(err, domain.ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got: %v", err)
	}
}

func TestRoleRepository_ListForOrganization_IncludesBuiltinAndCustom(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := rbacpg.NewRoleRepository(pool)
	orgID := insertOrg(t, root)

	if err := repo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles: %v", err)
	}
	custom, _ := domain.NewRole(orgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	if err := repo.Create(ctx, custom); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, custom.ID) })

	roles, err := repo.ListForOrganization(ctx, orgID)
	if err != nil {
		t.Fatalf("ListForOrganization: %v", err)
	}

	var sawBuiltinOwner, sawCustom bool
	for _, r := range roles {
		if r.OrganizationID == nil && r.Name == domain.RoleOwner {
			sawBuiltinOwner = true
		}
		if r.OrganizationID != nil && *r.OrganizationID == orgID && r.Name == "deployer" {
			sawCustom = true
		}
	}
	if !sawBuiltinOwner {
		t.Error("expected the built-in owner role to be visible")
	}
	if !sawCustom {
		t.Error("expected this org's own custom role to be visible")
	}
}
