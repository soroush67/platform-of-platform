package application

import (
	"context"

	"platform-of-platform/internal/variables/domain"
)

type VariableRepository interface {
	Create(ctx context.Context, v *domain.Variable) error
	// GetByScope returns domain.ErrVariableNotFound if no row exists for
	// this exact (scope_type, scope_id, key) - the single lookup the
	// resolution cascade calls once per scope level.
	GetByScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID, key string) (*domain.Variable, error)
	ListByScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID string) ([]*domain.Variable, error)
	// GetByID/Update/Delete are the direct-CRUD lookup shape (by the
	// Variable's own id, from the URL) - UpdateVariableService/
	// DeleteVariableService's own ports, previously entirely unbuilt
	// (this context only ever had create/list/resolve).
	GetByID(ctx context.Context, organizationID, variableID string) (*domain.Variable, error)
	Update(ctx context.Context, v *domain.Variable) error
	Delete(ctx context.Context, organizationID, variableID string) error
}

// ProjectChecker / EnvironmentChecker / WorkspaceChecker - this
// context's own ports into Tenancy/Workspace for "does this scope_id
// genuinely resolve to a real resource in this org" (scope_id for
// scope_type=organization is checked by simple string comparison
// against organizationID in the service itself, no cross-context call
// needed for that one case).
type ProjectChecker interface {
	ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error)
}

// OrganizationChecker - a separate concern from the scope-existence
// checkers above: "is this Organization archived" (docs/architecture/
// 13-module-identity-rbac-tenancy.md §1). CreateVariableService checks
// this before creating a new Variable, the same enforcement point
// tenancy.CreateProjectService/workspace.CreateWorkspaceService apply.
type OrganizationChecker interface {
	IsArchived(ctx context.Context, organizationID string) (bool, error)
}

type EnvironmentChecker interface {
	Exists(ctx context.Context, organizationID, environmentID string) (bool, error)
}

type WorkspaceChecker interface {
	Exists(ctx context.Context, organizationID, workspaceID string) (bool, error)
	// GetScope is used only by ResolveVariableService, to walk
	// Workspace -> Environment -> Project -> Organization
	// (docs/architecture/03-domain-model.md §7's cascade).
	GetScope(ctx context.Context, organizationID, workspaceID string) (projectID string, environmentID *string, err error)
}

type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// VisibilityChecker - this context's own copy of Tenancy/Workspace/
// Execution's identically-shaped port (see Tenancy's ports.go for why
// this is a distinct primitive, not a naming variant of
// HasPermissionAtScope-style checks). ListVariablesService uses this to
// gate project/environment/workspace-scoped Variables the same way
// their owning Project is gated - organization-scoped Variables stay
// membership-only (genuinely org-wide config, not Project-specific).
type VisibilityChecker interface {
	HasScopedPermission(ctx context.Context, organizationID, userID, permission, scopeType, scopeID string) (bool, error)
}

// EnvironmentProjectResolver resolves an environment-scoped Variable's
// scope_id back to its owning Project id, the same way WorkspaceChecker.
// GetScope already does for workspace-scoped Variables - needed only for
// the new per-Project visibility gate (ListVariablesService), since
// Environment isn't otherwise looked up anywhere in this context.
// Environment.ProjectID already exists directly on the Workspace
// context's own Environment row - this is a thin resolver over that,
// wired in main.go the same *Func-adapts-a-method-value way
// SecretMountCheckerFunc below already does.
type EnvironmentProjectResolver interface {
	ProjectIDForEnvironment(ctx context.Context, organizationID, environmentID string) (string, error)
}

type EnvironmentProjectResolverFunc func(ctx context.Context, organizationID, environmentID string) (string, error)

func (f EnvironmentProjectResolverFunc) ProjectIDForEnvironment(ctx context.Context, organizationID, environmentID string) (string, error) {
	return f(ctx, organizationID, environmentID)
}

// SecretMountChecker - Variables never imports secrets/domain (this
// codebase's no-cross-context-import rule), so CreateVariableService
// verifies a secret_ref's mount_id resolves to a real
// secrets/domain.SecretMount in this org through this port instead,
// same shape as ProjectChecker/EnvironmentChecker/WorkspaceChecker
// above.
type SecretMountChecker interface {
	SecretMountExists(ctx context.Context, organizationID, mountID string) (bool, error)
}

// SecretMountCheckerFunc lets main.go adapt a method value straight into
// this port, matching this codebase's existing *CheckerFunc precedent
// (e.g. tenancy's ScopeValidatorFunc) instead of a dedicated adapter
// struct.
type SecretMountCheckerFunc func(ctx context.Context, organizationID, mountID string) (bool, error)

func (f SecretMountCheckerFunc) SecretMountExists(ctx context.Context, organizationID, mountID string) (bool, error) {
	return f(ctx, organizationID, mountID)
}

// SecretResolver is wired in main.go to secrets/application's own
// ResolveSecretService, whose ResolveValue method already matches this
// exact signature with zero adapter glue code needed - the same
// structural-satisfaction pattern this codebase already uses elsewhere
// (e.g. RoleBindingRepository satisfying multiple ports at once).
type SecretResolver interface {
	ResolveValue(ctx context.Context, organizationID, mountID, path string) (string, error)
}
