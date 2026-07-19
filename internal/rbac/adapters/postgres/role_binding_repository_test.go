package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	rbacpg "platform-of-platform/internal/rbac/adapters/postgres"
	"platform-of-platform/internal/rbac/domain"
)

func TestRoleBindingRepository_AssignRoleAndHasPermission(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	if err := roleRepo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles: %v", err)
	}

	if err := repo.AssignRole(ctx, orgID, userID, domain.RoleRead); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE organization_id = $1`, orgID) })

	allowed, err := repo.HasPermission(ctx, orgID, userID, string(domain.PermissionWorkspaceRead))
	if err != nil {
		t.Fatalf("HasPermission (read): %v", err)
	}
	if !allowed {
		t.Error("expected the 'read' role to grant workspace:read")
	}

	allowed, err = repo.HasPermission(ctx, orgID, userID, string(domain.PermissionWorkspaceManage))
	if err != nil {
		t.Fatalf("HasPermission (manage): %v", err)
	}
	if allowed {
		t.Error("expected the 'read' role NOT to grant workspace:manage")
	}
}

func TestRoleBindingRepository_ReplaceRole_ReplacesNotAdds(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	if err := roleRepo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles: %v", err)
	}
	if err := repo.AssignRole(ctx, orgID, userID, domain.RoleRead); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE organization_id = $1`, orgID) })

	if err := repo.ReplaceRole(ctx, orgID, userID, domain.RoleWrite); err != nil {
		t.Fatalf("ReplaceRole: %v", err)
	}

	allowed, err := repo.HasPermission(ctx, orgID, userID, string(domain.PermissionWorkspaceManage))
	if err != nil {
		t.Fatalf("HasPermission (manage, after replace): %v", err)
	}
	if !allowed {
		t.Error("expected the new 'write' role to grant workspace:manage")
	}

	// root, not pool - role_bindings has FORCE ROW LEVEL SECURITY, and a
	// raw verification query on the app pool with no app.current_org_id
	// set would silently see zero rows (found for real: this test's
	// first version used pool here and failed with "got 0", not because
	// ReplaceRole was broken, but because this query itself was RLS-blind).
	var count int
	if err := root.QueryRow(ctx,
		`SELECT count(*) FROM role_bindings WHERE organization_id = $1 AND subject_id = $2 AND scope_type = 'organization'`,
		orgID, userID,
	).Scan(&count); err != nil {
		t.Fatalf("query role_bindings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 organization-scope binding after ReplaceRole (a real replace, not an add), got %d", count)
	}
}

// TestRoleBindingRepository_GetOrgScopeRoleName proves the query
// GetOrgScopeRoleName runs (satisfying Tenancy's own RoleReader port
// for the member roster) actually tracks reality through an unbound ->
// bound -> replaced lifecycle, not just that it parses.
func TestRoleBindingRepository_GetOrgScopeRoleName(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	if err := roleRepo.SeedBuiltinRoles(ctx); err != nil {
		t.Fatalf("SeedBuiltinRoles: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE organization_id = $1`, orgID) })

	_, found, err := repo.GetOrgScopeRoleName(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("GetOrgScopeRoleName (before any binding): %v", err)
	}
	if found {
		t.Error("expected found=false before any org-scope binding exists")
	}

	if err := repo.AssignRole(ctx, orgID, userID, domain.RoleRead); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	name, found, err := repo.GetOrgScopeRoleName(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("GetOrgScopeRoleName (after AssignRole): %v", err)
	}
	if !found || name != domain.RoleRead {
		t.Errorf("expected found=true name=%q, got found=%v name=%q", domain.RoleRead, found, name)
	}

	if err := repo.ReplaceRole(ctx, orgID, userID, domain.RoleWrite); err != nil {
		t.Fatalf("ReplaceRole: %v", err)
	}
	name, found, err = repo.GetOrgScopeRoleName(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("GetOrgScopeRoleName (after ReplaceRole): %v", err)
	}
	if !found || name != domain.RoleWrite {
		t.Errorf("expected the replaced role %q to be reflected, got found=%v name=%q", domain.RoleWrite, found, name)
	}
}

func TestRoleBindingRepository_HasPermissionAtScope_ProjectScopeGrant(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	projectID := uuid.NewString()
	binding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeProject, projectID, domain.EffectAllow)
	if err := repo.Create(ctx, binding); err != nil {
		t.Fatalf("Create binding: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE id = $1`, binding.ID) })

	allowed, err := repo.HasPermissionAtScope(ctx, orgID, userID, string(domain.PermissionWorkspaceApply), &projectID, nil)
	if err != nil {
		t.Fatalf("HasPermissionAtScope (matching project): %v", err)
	}
	if !allowed {
		t.Error("expected a project-scope binding to grant the permission when checked at that same project")
	}

	otherProjectID := uuid.NewString()
	allowed, err = repo.HasPermissionAtScope(ctx, orgID, userID, string(domain.PermissionWorkspaceApply), &otherProjectID, nil)
	if err != nil {
		t.Fatalf("HasPermissionAtScope (different project): %v", err)
	}
	if allowed {
		t.Error("expected a project-scope binding NOT to grant the permission at a different project")
	}
}

// TestRoleBindingRepository_HasScopedPermission_NoOrganizationFallback is
// HasScopedPermission's own defining behavior, the whole reason it exists
// as a separate method from HasPermissionAtScope above: an organization-
// scope binding must NOT grant it, even though HasPermissionAtScope
// treats that same binding as matching every project unconditionally
// (see project_visibility.go in Tenancy/Workspace/Execution/Variables'
// own application packages for why - every org member already gets an
// organization-scope "read" role binding on join, so reusing
// HasPermissionAtScope for a real default-hidden Project visibility
// gate would never actually hide anything).
func TestRoleBindingRepository_HasScopedPermission_NoOrganizationFallback(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "reader", []domain.Permission{domain.PermissionProjectRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	projectID := uuid.NewString()
	orgBinding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, orgBinding); err != nil {
		t.Fatalf("Create org-scope binding: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE id = $1`, orgBinding.ID) })

	// The exact same organization-scope binding that DOES satisfy
	// HasPermissionAtScope for this project (proving this isn't just a
	// bug in the fixture) must NOT satisfy HasScopedPermission.
	allowedAtScope, err := repo.HasPermissionAtScope(ctx, orgID, userID, string(domain.PermissionProjectRead), &projectID, nil)
	if err != nil {
		t.Fatalf("HasPermissionAtScope: %v", err)
	}
	if !allowedAtScope {
		t.Fatal("expected HasPermissionAtScope to treat the org-scope binding as matching this project (sanity check)")
	}

	allowedScoped, err := repo.HasScopedPermission(ctx, orgID, userID, string(domain.PermissionProjectRead), "project", projectID)
	if err != nil {
		t.Fatalf("HasScopedPermission: %v", err)
	}
	if allowedScoped {
		t.Error("expected HasScopedPermission NOT to fall back to the organization-scope binding")
	}

	// Now bind the same role directly at this project's own scope -
	// HasScopedPermission should allow it once the grant is genuinely
	// that specific.
	projectBinding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeProject, projectID, domain.EffectAllow)
	if err := repo.Create(ctx, projectBinding); err != nil {
		t.Fatalf("Create project-scope binding: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE id = $1`, projectBinding.ID) })

	allowedScoped, err = repo.HasScopedPermission(ctx, orgID, userID, string(domain.PermissionProjectRead), "project", projectID)
	if err != nil {
		t.Fatalf("HasScopedPermission (after project-scope grant): %v", err)
	}
	if !allowedScoped {
		t.Error("expected an exact project-scope binding to satisfy HasScopedPermission")
	}

	// A different project's id must still not match.
	otherProjectID := uuid.NewString()
	allowedScoped, err = repo.HasScopedPermission(ctx, orgID, userID, string(domain.PermissionProjectRead), "project", otherProjectID)
	if err != nil {
		t.Fatalf("HasScopedPermission (different project): %v", err)
	}
	if allowedScoped {
		t.Error("expected the project-scope binding NOT to satisfy HasScopedPermission for a different project id")
	}
}

// TestRoleBindingRepository_HasPermissionAtScope_DenyOverridesAllow is
// the real regression test for HasPermissionAtScope's own doc comment -
// the entire reason this codebase's RBAC evaluation is AWS-IAM-style
// (collect every matching binding, any Deny wins unconditionally) and
// not Kubernetes RBAC's pure-additive model: an org-wide Allow must be
// genuinely narrowable by a single workspace-scope Deny for the exact
// same permission, while staying intact at every workspace the Deny
// doesn't name.
func TestRoleBindingRepository_HasPermissionAtScope_DenyOverridesAllow(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "writer", []domain.Permission{domain.PermissionWorkspaceApply})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	orgAllow := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, orgAllow); err != nil {
		t.Fatalf("Create org allow: %v", err)
	}

	deniedWorkspaceID := uuid.NewString()
	workspaceDeny := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeWorkspace, deniedWorkspaceID, domain.EffectDeny)
	if err := repo.Create(ctx, workspaceDeny); err != nil {
		t.Fatalf("Create workspace deny: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE organization_id = $1`, orgID) })

	projectID := uuid.NewString()
	allowed, err := repo.HasPermissionAtScope(ctx, orgID, userID, string(domain.PermissionWorkspaceApply), &projectID, &deniedWorkspaceID)
	if err != nil {
		t.Fatalf("HasPermissionAtScope (denied workspace): %v", err)
	}
	if allowed {
		t.Error("expected the workspace-scope Deny to override the org-wide Allow at that specific workspace")
	}

	otherWorkspaceID := uuid.NewString()
	allowed, err = repo.HasPermissionAtScope(ctx, orgID, userID, string(domain.PermissionWorkspaceApply), &projectID, &otherWorkspaceID)
	if err != nil {
		t.Fatalf("HasPermissionAtScope (other workspace): %v", err)
	}
	if !allowed {
		t.Error("expected the org-wide Allow to still apply at a workspace the Deny doesn't name")
	}
}

func TestRoleBindingRepository_HasPermissionAtScope_TeamMediatedBinding(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	teamID := uuid.NewString()
	mustExec(t, root, `INSERT INTO teams (id, organization_id, name) VALUES ($1, $2, 'platform-team')`, teamID, orgID)
	mustExec(t, root, `INSERT INTO team_memberships (id, team_id, organization_id, user_id) VALUES ($1, $2, $3, $4)`, uuid.NewString(), teamID, orgID, userID)
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM team_memberships WHERE team_id = $1`, teamID)
		mustExec(t, root, `DELETE FROM teams WHERE id = $1`, teamID)
	})

	role, _ := domain.NewRole(orgID, "team-role", []domain.Permission{domain.PermissionWorkspaceRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	binding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeTeam, teamID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, binding); err != nil {
		t.Fatalf("Create binding: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE id = $1`, binding.ID) })

	allowed, err := repo.HasPermission(ctx, orgID, userID, string(domain.PermissionWorkspaceRead))
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !allowed {
		t.Error("expected a team-scoped binding to grant the permission to a user who's a member of that team")
	}
}

func TestRoleBindingRepository_Create_DuplicateIsIgnored(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "dup-role", []domain.Permission{domain.PermissionWorkspaceRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	binding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, binding); err != nil {
		t.Fatalf("Create (first): %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE role_id = $1`, role.ID) })

	// Same (role_id, subject_type, subject_id, scope_type, scope_id) key,
	// a fresh id - Create's own ON CONFLICT DO NOTHING must silently
	// no-op, not error.
	duplicate := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, duplicate); err != nil {
		t.Fatalf("Create (duplicate): %v", err)
	}

	// root, not pool - same RLS-blind-query reasoning as
	// TestRoleBindingRepository_ReplaceRole_ReplacesNotAdds above.
	var count int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE role_id = $1`, role.ID).Scan(&count); err != nil {
		t.Fatalf("query role_bindings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 binding row despite two Create calls with the same key, got %d", count)
	}
}

func TestRoleBindingRepository_Delete(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "delete-target-role", []domain.Permission{domain.PermissionWorkspaceRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	binding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, binding); err != nil {
		t.Fatalf("Create binding: %v", err)
	}

	if err := repo.Delete(ctx, orgID, binding.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE id = $1`, binding.ID).Scan(&count); err != nil {
		t.Fatalf("query role_bindings: %v", err)
	}
	if count != 0 {
		t.Error("expected the binding row to be gone after Delete")
	}
}

func TestRoleBindingRepository_Delete_WrongOrganizationLeavesRowIntact(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "delete-wrong-org-role", []domain.Permission{domain.PermissionWorkspaceRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	binding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, binding); err != nil {
		t.Fatalf("Create binding: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE id = $1`, binding.ID) })

	// A mismatched organization_id in the WHERE clause must match zero
	// rows - the real, second enforcement point beyond the application
	// layer's own membership/permission checks.
	if err := repo.Delete(ctx, uuid.NewString(), binding.ID); err != nil {
		t.Fatalf("Delete (wrong org): %v", err)
	}

	var count int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE id = $1`, binding.ID).Scan(&count); err != nil {
		t.Fatalf("query role_bindings: %v", err)
	}
	if count != 1 {
		t.Error("expected the binding row to survive a Delete scoped to a different organization")
	}
}

func TestRoleBindingRepository_ListForSubject(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userA := insertUser(t, root)
	userB := insertUser(t, root)

	role, _ := domain.NewRole(orgID, "list-role", []domain.Permission{domain.PermissionWorkspaceRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	bindingA := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userA, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	bindingB := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userB, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, bindingA); err != nil {
		t.Fatalf("Create bindingA: %v", err)
	}
	if err := repo.Create(ctx, bindingB); err != nil {
		t.Fatalf("Create bindingB: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE role_id = $1`, role.ID) })

	onlyA, err := repo.ListForSubject(ctx, orgID, userA)
	if err != nil {
		t.Fatalf("ListForSubject (userA): %v", err)
	}
	if len(onlyA) != 1 || onlyA[0].SubjectID != userA {
		t.Errorf("expected exactly userA's own binding, got %+v", onlyA)
	}

	all, err := repo.ListForSubject(ctx, orgID, "")
	if err != nil {
		t.Fatalf("ListForSubject (all): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected both bindings when subjectID is empty, got %d", len(all))
	}
}

func TestRoleBindingRepository_DeleteForSubject(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	roleRepo := rbacpg.NewRoleRepository(pool)
	repo := rbacpg.NewRoleBindingRepository(pool)
	orgID := insertOrg(t, root)
	userID := insertUser(t, root)
	teamID := uuid.NewString()
	mustExec(t, root, `INSERT INTO teams (id, organization_id, name) VALUES ($1, $2, 'delete-for-subject-team')`, teamID, orgID)
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM teams WHERE id = $1`, teamID) })

	role, _ := domain.NewRole(orgID, "delete-for-subject-role", []domain.Permission{domain.PermissionWorkspaceRead})
	if err := roleRepo.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM roles WHERE id = $1`, role.ID) })

	teamBinding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeTeam, teamID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, teamBinding); err != nil {
		t.Fatalf("Create team binding: %v", err)
	}
	userBinding := domain.NewRoleBinding(orgID, role.ID, domain.SubjectTypeUser, userID, domain.ScopeTypeOrganization, orgID, domain.EffectAllow)
	if err := repo.Create(ctx, userBinding); err != nil {
		t.Fatalf("Create user binding: %v", err)
	}
	t.Cleanup(func() { mustExec(t, root, `DELETE FROM role_bindings WHERE role_id = $1`, role.ID) })

	if err := repo.DeleteForSubject(ctx, orgID, "team", teamID); err != nil {
		t.Fatalf("DeleteForSubject: %v", err)
	}

	var teamCount, userCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE id = $1`, teamBinding.ID).Scan(&teamCount); err != nil {
		t.Fatalf("query team binding: %v", err)
	}
	if err := root.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE id = $1`, userBinding.ID).Scan(&userCount); err != nil {
		t.Fatalf("query user binding: %v", err)
	}
	if teamCount != 0 {
		t.Error("expected the team's own binding to be gone")
	}
	if userCount != 1 {
		t.Error("expected the unrelated user binding to survive - DeleteForSubject must only touch the exact subject asked for")
	}
}
