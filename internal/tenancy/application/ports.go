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
	// Create takes createdByUserID purely to attach it to the
	// OrganizationCreated outbox event it writes in the same
	// transaction as the INSERT (internal/platform/outbox's whole
	// reason to exist) - it's not a field on Organization itself
	// (Stage 3 §2 has no created_by column), so the application layer
	// can't just read it back off org afterward.
	Create(ctx context.Context, org *domain.Organization, createdByUserID string) error
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

// RoleChanger is distinct from RoleAssigner - AssignRole is additive
// (bootstrapping the first role a member ever gets, at org-creation or
// add-member time), RoleChanger.ReplaceRole is real "this member now has
// exactly this role, not that one" semantics (see the rbac postgres
// adapter's own comment on why AssignRole alone can't safely be reused
// for a change operation).
type RoleChanger interface {
	ReplaceRole(ctx context.Context, organizationID, userID, roleName string) error
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// RootMembershipRepository is a deliberately separate port from
// MembershipRepository above - ListOrganizationsForUser ("every org
// this user belongs to") is a genuine cross-org read with no single
// organization_id to scope RLS to in advance (organization_memberships
// and organizations both have FORCE ROW LEVEL SECURITY, scoped by
// app.current_org_id, per migrations/0001_init.up.sql) - the normal
// app-pool-backed MembershipRepository would silently see zero rows,
// not error. This needs the same root-connection exception
// internal/platform/idempotency/reaper.go's own doc comment already
// establishes for exactly this class of problem (idempotency_keys also
// has FORCE RLS and is also genuinely cross-org). Safe specifically
// because the query is always filtered by the caller's own JWT-derived
// user_id (httpserver.UserIDFromContext), never a client-supplied
// value - it cannot be used to enumerate any other user's org
// memberships, unlike a hypothetical `GET /orgs?user_id=` would be.
type RootMembershipRepository interface {
	ListOrganizationsForUser(ctx context.Context, userID string) ([]*domain.Organization, error)
}

// ProjectRepository - same shape/reasoning as OrganizationRepository.
type ProjectRepository interface {
	Create(ctx context.Context, project *domain.Project) error
	// GetByID returns ErrProjectNotFound if no row is visible - RLS scopes
	// this to organizationID first (see the postgres adapter), so a
	// project id from a *different* org than the one in the URL is
	// indistinguishable from a nonexistent id, same "don't reveal
	// existence" posture as everywhere else in this codebase.
	GetByID(ctx context.Context, organizationID, id string) (*domain.Project, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Project, error)
}
