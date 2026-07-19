package application

import "context"

// permissionProjectRead/permissionWorkspaceRead - this context's own
// copies of the same permission name strings Tenancy/RBAC declare,
// matching this codebase's existing per-context constant-redeclaration
// style (e.g. permissionOrganizationManage above, permissionWorkspaceManage
// in create_workspace.go).
const (
	permissionProjectRead   = "project:read"
	permissionWorkspaceRead = "workspace:read"
)

// canAccessProject/canAccessWorkspace are the shared visibility gates
// ListWorkspacesService/GetWorkspaceService compose - see Tenancy's own
// project_visibility.go and VisibilityChecker's doc comment (ports.go)
// for why HasScopedPermission (not the existing HasPermissionAtScope)
// is the right primitive here: an organization-scope RoleBinding must
// NOT make a Project/Workspace visible by itself (every member already
// has one via the builtin "read" role), only an Owner/Admin bypass or an
// explicit grant at that Project's (or, narrower, that Workspace's) own
// scope should.
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

// canAccessWorkspace is canAccessProject widened with a narrower
// workspace-scope OR-branch, for granting just one Workspace without
// handing out the whole Project.
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
