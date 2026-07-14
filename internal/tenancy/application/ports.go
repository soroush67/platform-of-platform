package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// OrganizationRepository is the port the /adapters/postgres package
// satisfies - this package declares the interface shaped for its own
// use, per the dependency-inversion rule in
// docs/architecture/18-backend-structure.md §3.
type OrganizationRepository interface {
	Create(ctx context.Context, org *domain.Organization) error
	// GetByID returns ErrOrganizationNotFound if no row is visible for id -
	// either because it genuinely doesn't exist, or because RLS hid it
	// (the two are indistinguishable by design, per
	// docs/architecture/05-database.md §1 - a 404 here reveals nothing
	// about whether some *other* org's id happens to exist).
	GetByID(ctx context.Context, id string) (*domain.Organization, error)
}

// MembershipRepository is the port for OrganizationMembership -
// deliberately its own interface, not folded into OrganizationRepository,
// since it's a different aggregate/entity with different access patterns
// (docs/architecture/03-domain-model.md §2).
type MembershipRepository interface {
	Create(ctx context.Context, membership *domain.OrganizationMembership) error
	// IsMember answers the actual access-control question every
	// org-scoped read/write needs: not "does this org exist" (that's
	// OrganizationRepository's RLS, which - as GetOrganizationService's
	// own comment documents - is trivially satisfiable by anyone who
	// knows the id) but "is *this specific* user allowed to see it."
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

// RoleAssigner and PermissionChecker are Tenancy's own ports into the
// RBAC context, shaped for exactly what Tenancy needs - dependency
// inversion per docs/architecture/18-backend-structure.md §3: Tenancy
// doesn't import internal/rbac/domain at all, it declares the interface
// it needs and the rbac postgres adapter happens to satisfy it
// structurally (Go interfaces, no explicit "implements" wiring needed).
type RoleAssigner interface {
	AssignRole(ctx context.Context, organizationID, userID, roleName string) error
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}
