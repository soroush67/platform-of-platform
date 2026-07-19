// Package application is the RBAC context's use-case layer - didn't
// exist before this slice (every gated action in every *other* context
// called straight into the postgres adapter's HasPermission/AssignRole/
// ReplaceRole as a cross-context port). This layer exists specifically
// for RBAC's *own* first-class endpoints (custom roles, generic
// role-bindings) - docs/architecture/13-module-identity-rbac-tenancy.md
// §3, previously entirely unbuilt.
package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

type RoleRepository interface {
	Create(ctx context.Context, role *domain.Role) error
	ListForOrganization(ctx context.Context, organizationID string) ([]*domain.Role, error)
	GetByID(ctx context.Context, organizationID, roleID string) (*domain.Role, error)
	// Update rewrites a custom Role's permission set in place
	// (UpdateRoleService's own doc comment on why only a *custom* Role,
	// never a builtin one, can reach this) - name stays immutable, only
	// permissions change, so this never has to handle the unique-name-
	// conflict case Create's own INSERT does.
	Update(ctx context.Context, role *domain.Role) error
}

type RoleBindingRepository interface {
	Create(ctx context.Context, binding *domain.RoleBinding) error
	ListForSubject(ctx context.Context, organizationID, subjectID string) ([]*domain.RoleBinding, error)
	// Delete backs DeleteRoleBindingService - a real, permanent removal
	// (operator-confirmed, not a soft/client-side hide).
	Delete(ctx context.Context, organizationID, bindingID string) error
}

// MembershipChecker/PermissionChecker - this context's own copies of the
// same port shape every other context declares locally
// (docs/architecture/18-backend-structure.md §3's dependency-inversion
// rule - no shared cross-context interface package).
type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// ProjectChecker/WorkspaceChecker/TeamChecker are what
// CreateRoleBindingService uses to validate scope_id/subject_id actually
// point at a real resource in this org before creating a grant for it -
// same "validate before writing" posture as every other cross-context
// check in this codebase.
type ProjectChecker interface {
	ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error)
}

// WorkspaceChecker uses the projectID-free existence check
// (WorkspaceRepository.Exists) - a POST /role-bindings request scoped to
// a workspace only carries {scope: {type: "workspace", id: "..."}}, no
// project_id, so this can't require one the way execution's own
// WorkspaceChecker port does.
type WorkspaceChecker interface {
	Exists(ctx context.Context, organizationID, workspaceID string) (bool, error)
}

type TeamChecker interface {
	TeamExists(ctx context.Context, organizationID, teamID string) (bool, error)
}

// ServiceAccountChecker validates a subject_type='service_account'
// binding's subject.id actually points at a real ServiceAccount in this
// org (internal/identity/domain/service_account.go) - same "validate
// before writing" posture as TeamChecker above.
type ServiceAccountChecker interface {
	ServiceAccountExists(ctx context.Context, organizationID, serviceAccountID string) (bool, error)
}

// UserReader/TeamNameReader/ServiceAccountNameReader/ProjectNameReader/
// WorkspaceNameReader back ListRoleBindingsService's own display-name
// resolution (list_role_bindings.go) - a role binding's subject_id/
// scope_id are otherwise opaque UUIDs to whoever's reading the list in
// the UI. Each is a narrow read-only lookup into another context,
// same per-context port-declaration convention as the checkers above -
// found=false (not an error) is what a since-deleted or unresolvable
// row maps to, matching ListMembersService's own "an emptier row, not a
// failed roster" reasoning (internal/tenancy/application/list_members.go).
// UserReader's signature is deliberately identical to Tenancy's own
// UserReader port - both are satisfied by the same identity/adapters/
// postgres.UserRepository.GetUser method, reused here with zero glue.
type UserReader interface {
	GetUser(ctx context.Context, userID string) (username, email string, found bool, err error)
}

type TeamNameReader interface {
	GetTeamName(ctx context.Context, organizationID, teamID string) (name string, found bool, err error)
}

// TeamNameReaderFunc/ServiceAccountNameReaderFunc/ProjectNameReaderFunc/
// WorkspaceNameReaderFunc let main.go adapt a closure straight into each
// port, matching this codebase's existing *CheckerFunc/*ResolverFunc
// precedent (e.g. variables/application's own SecretMountCheckerFunc,
// EnvironmentProjectResolverFunc) instead of a dedicated adapter struct -
// each closure wraps another context's GetByID and translates its
// not-found error into (_, false, nil).
type TeamNameReaderFunc func(ctx context.Context, organizationID, teamID string) (string, bool, error)

func (f TeamNameReaderFunc) GetTeamName(ctx context.Context, organizationID, teamID string) (string, bool, error) {
	return f(ctx, organizationID, teamID)
}

type ServiceAccountNameReader interface {
	GetServiceAccountName(ctx context.Context, organizationID, serviceAccountID string) (name string, found bool, err error)
}

type ServiceAccountNameReaderFunc func(ctx context.Context, organizationID, serviceAccountID string) (string, bool, error)

func (f ServiceAccountNameReaderFunc) GetServiceAccountName(ctx context.Context, organizationID, serviceAccountID string) (string, bool, error) {
	return f(ctx, organizationID, serviceAccountID)
}

type ProjectNameReader interface {
	GetProjectName(ctx context.Context, organizationID, projectID string) (name string, found bool, err error)
}

type ProjectNameReaderFunc func(ctx context.Context, organizationID, projectID string) (string, bool, error)

func (f ProjectNameReaderFunc) GetProjectName(ctx context.Context, organizationID, projectID string) (string, bool, error) {
	return f(ctx, organizationID, projectID)
}

type WorkspaceNameReader interface {
	GetWorkspaceName(ctx context.Context, organizationID, workspaceID string) (name string, found bool, err error)
}

type WorkspaceNameReaderFunc func(ctx context.Context, organizationID, workspaceID string) (string, bool, error)

func (f WorkspaceNameReaderFunc) GetWorkspaceName(ctx context.Context, organizationID, workspaceID string) (string, bool, error) {
	return f(ctx, organizationID, workspaceID)
}
