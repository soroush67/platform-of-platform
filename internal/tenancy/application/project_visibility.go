package application

import "context"

// canAccessProject is the shared visibility gate ListProjectsService and
// GetProjectService both compose: an Owner/Admin (organization:manage)
// bypasses it and sees every Project, same as before this change;
// everyone else needs a real RoleBinding (direct or via a Team) granting
// project:read at exactly this Project's own scope - see
// VisibilityChecker's own doc comment (ports.go) for why this can't
// reuse the existing org-wide PermissionChecker/HasPermissionAtScope
// checks every other permission in this codebase uses.
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
