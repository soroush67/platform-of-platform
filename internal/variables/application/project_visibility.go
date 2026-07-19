package application

import "context"

// permissionProjectRead/permissionWorkspaceRead - this context's own
// copies of the same permission name strings Tenancy/Workspace/RBAC
// declare, matching this codebase's existing per-context constant-
// redeclaration style (e.g. permissionOrganizationManage in
// create_variable.go).
const (
	permissionProjectRead   = "project:read"
	permissionWorkspaceRead = "workspace:read"
)

// canAccessProject/canAccessWorkspace - same shared visibility gates as
// Workspace/Execution's own project_visibility.go (an Owner/Admin
// bypasses it, everyone else needs a real project- or workspace-scope
// grant, direct or via a Team). Used only for project/environment/
// workspace-scoped Variables - organization-scoped Variables stay
// membership-only (ListVariablesService.Execute below), genuinely
// org-wide config rather than Project-specific.
func canAccessProject(ctx context.Context, permChecker PermissionChecker, visibilityChecker VisibilityChecker, organizationID, userID, projectID string) (bool, error) {
	isAdmin, err := permChecker.HasPermission(ctx, organizationID, userID, permissionOrganizationManage)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}

	return visibilityChecker.HasScopedPermission(ctx, organizationID, userID, permissionProjectRead, "project", projectID)
}

func canAccessWorkspace(ctx context.Context, permChecker PermissionChecker, visibilityChecker VisibilityChecker, organizationID, userID, projectID, workspaceID string) (bool, error) {
	canAccess, err := canAccessProject(ctx, permChecker, visibilityChecker, organizationID, userID, projectID)
	if err != nil {
		return false, err
	}
	if canAccess {
		return true, nil
	}

	return visibilityChecker.HasScopedPermission(ctx, organizationID, userID, permissionWorkspaceRead, "workspace", workspaceID)
}
