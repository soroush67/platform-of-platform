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
}

type RoleBindingRepository interface {
	Create(ctx context.Context, binding *domain.RoleBinding) error
	ListForSubject(ctx context.Context, organizationID, subjectID string) ([]*domain.RoleBinding, error)
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
