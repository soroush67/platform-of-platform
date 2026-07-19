package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

// EnvironmentRepository / WorkspaceRepository - same shape/reasoning as
// every other context's *Repository port in this codebase.
type EnvironmentRepository interface {
	Create(ctx context.Context, env *domain.Environment) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Environment, error)
	ListByProject(ctx context.Context, organizationID, projectID string) ([]*domain.Environment, error)
}

type WorkspaceRepository interface {
	Create(ctx context.Context, ws *domain.Workspace) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Workspace, error)
	ListByProject(ctx context.Context, organizationID, projectID string) ([]*domain.Workspace, error)
}

// MembershipChecker and PermissionChecker are this context's own ports
// into Tenancy/RBAC, declared here rather than imported from there
// (docs/architecture/18-backend-structure.md §3 - identical reasoning,
// and an identical method signature, to Tenancy's own ports into RBAC;
// each context still declares its own copy rather than sharing one
// "common" port type, so no context ever depends on a cross-cutting
// port package that isn't really cross-cutting).
type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// VisibilityChecker - this context's own copy of Tenancy's identically-
// shaped port (ports.go there has its own doc comment on why this is a
// distinct primitive from ScopedPermissionChecker/HasPermissionAtScope
// below, not just a naming variant of it).
type VisibilityChecker interface {
	HasScopedPermission(ctx context.Context, organizationID, userID, permission, scopeType, scopeID string) (bool, error)
}

// ScopedPermissionChecker - same reasoning as execution's own copy
// (internal/execution/application/ports.go): a RoleBinding at project
// scope should grant workspace:manage for Workspaces/Environments
// created under that project, not just an organization-scope binding.
// CreateWorkspaceService/CreateEnvironmentService pass projectID with a
// nil workspaceID (the Workspace/Environment doesn't exist yet at the
// point permission is checked - there's nothing narrower than project
// to check against).
type ScopedPermissionChecker interface {
	HasPermissionAtScope(ctx context.Context, organizationID, userID, permission string, projectID, workspaceID *string) (bool, error)
}

// ProjectChecker is this context's port into Tenancy specifically for
// "does this project genuinely belong to this org" - Workspace/
// Environment both reference project_id, and that reference needs to be
// verified against Tenancy's own data, not just trusted from the URL,
// the same "don't trust what the client typed" reasoning already applied
// throughout this codebase (e.g. GetOrganizationService's membership
// check instead of trusting the URL's org id).
type ProjectChecker interface {
	ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error)
}

// OrganizationChecker - this context's port into Tenancy for "is this
// Organization archived" (docs/architecture/13-module-identity-rbac-
// tenancy.md §1). CreateWorkspaceService checks this before creating a
// new Workspace - an archived org can't grow new structure, the same
// enforcement point tenancy.CreateProjectService already applies to
// Projects.
type OrganizationChecker interface {
	IsArchived(ctx context.Context, organizationID string) (bool, error)
}
