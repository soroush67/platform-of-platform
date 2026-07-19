package application

import "context"

// permissionOrganizationManage/permissionProjectRead/permissionWorkspaceRead
// - this context's own copies of the same permission name strings
// Tenancy/Workspace/RBAC declare, matching this codebase's existing
// per-context constant-redeclaration style (e.g. permissionWorkspaceApply
// in trigger_run.go).
const (
	permissionOrganizationManage = "organization:manage"
	permissionProjectRead        = "project:read"
	permissionWorkspaceRead      = "workspace:read"
)

// canAccessProject/canAccessWorkspace are the shared visibility gates
// ListRunsService/GetRunService compose - same reasoning as Workspace's
// own project_visibility.go (an Owner/Admin bypasses it, everyone else
// needs a real project- or workspace-scope grant, direct or via a Team;
// see VisibilityChecker's own doc comment, ports.go, for why
// HasScopedPermission - not the existing HasPermissionAtScope
// trigger_run.go/cancel_run.go already use - is the right primitive).
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
