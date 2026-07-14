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
}

// ProjectChecker / EnvironmentChecker / WorkspaceChecker - this
// context's own ports into Tenancy/Workspace for "does this scope_id
// genuinely resolve to a real resource in this org" (there's no
// OrganizationChecker: scope_id for scope_type=organization is checked
// by simple string comparison against organizationID in the service
// itself, no cross-context call needed for that one case).
type ProjectChecker interface {
	ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error)
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
