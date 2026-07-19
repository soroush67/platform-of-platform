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
	// ListByOrganization backs the member roster (ListMembersService) -
	// same shape/reasoning as ProjectRepository.ListByOrganization below.
	// Deliberately NOT filtered by blocked_at - an admin must still see
	// a blocked member (with a real "blocked" badge) to be able to
	// unblock them.
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.OrganizationMembership, error)
	// MembershipExists is IsMember's blocked-agnostic sibling - target-
	// validation for Block/Unblock/RemoveMember needs "does this
	// membership row exist at all," not "is it currently usable" (IsMember
	// would incorrectly say a blocked target isn't a member at all,
	// making them impossible to ever unblock or remove through their own
	// target-validation check).
	MembershipExists(ctx context.Context, organizationID, userID string) (bool, error)
	// SetBlocked backs BlockMemberService/UnblockMemberService.
	SetBlocked(ctx context.Context, organizationID, userID string, blocked bool) error
	// Delete backs RemoveMemberService - a real, permanent removal of
	// this membership.
	Delete(ctx context.Context, organizationID, userID string) error
}

// UserReader and RoleReader are Tenancy's own ports into Identity and
// RBAC respectively, both shaped like RoleAssigner/RoleChanger below -
// primitive-only signatures, not a shared struct, so neither adapter
// package needs to import tenancy/application to construct one (the
// same dependency-inversion reasoning RoleAssigner's own doc comment
// gives). ListMembersService resolves each roster row's username/email
// and current org-scope role one call at a time (no bulk/batched
// cross-context port exists anywhere in this codebase yet) - org
// rosters aren't expected to be large enough for the N+1 shape to
// matter; a real, honest cost of staying consistent with the existing
// pattern rather than inventing a new one.
type UserReader interface {
	GetUser(ctx context.Context, userID string) (username, email string, found bool, err error)
	// ListAll returns every platform User (id, username, email) - backs
	// ListAvailableUsersService's "which Users aren't in this org yet"
	// computation. Anonymous struct, not a named type, so the identity
	// adapter satisfying this doesn't need to import this package -
	// same structural-typing trick GetUser's primitive-only signature
	// above already uses to dodge the cross-context import ban.
	ListAll(ctx context.Context) ([]struct{ ID, Username, Email string }, error)
}

// RoleReader.GetOrgScopeRoleName mirrors RoleChanger.ReplaceRole's own
// "there is exactly one org-scope role_binding per member" reasoning -
// found=false (not an error) when a member has none, which can
// genuinely happen for a member added outside AddMemberService's own
// AssignRole call.
type RoleReader interface {
	GetOrgScopeRoleName(ctx context.Context, organizationID, userID string) (roleName string, found bool, err error)
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

// RoleBindingCleaner is Tenancy's port into RBAC's own
// RoleBindingRepository.DeleteForSubject - DeleteTeamService/
// RemoveMemberService both call this before the subject itself
// (a Team row, or an organization_memberships row) is gone, so no
// RoleBinding is ever left dangling, pointing at a Team or a User no
// longer in this org.
type RoleBindingCleaner interface {
	DeleteForSubject(ctx context.Context, organizationID, subjectType, subjectID string) error
}

// VisibilityChecker backs the new per-Project visibility gate
// (ListProjectsService/GetProjectService) - deliberately a different
// port than PermissionChecker/ScopedPermissionChecker-style checks used
// elsewhere: HasScopedPermission (internal/rbac/adapters/postgres/
// role_binding_repository.go) has no organization-scope fallback, so a
// Team/User must be bound at exactly this Project's own scope to pass -
// see canAccessProject in list_projects.go for the Owner/Admin bypass
// this is composed with.
type VisibilityChecker interface {
	HasScopedPermission(ctx context.Context, organizationID, userID, permission, scopeType, scopeID string) (bool, error)
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
	// CountOrganizations backs CreateOrganizationService's own first-
	// org-ever bootstrap check - a genuine cross-org COUNT(*), same root-
	// connection exception as ListOrganizationsForUser above (organizations
	// has FORCE ROW LEVEL SECURITY, so the normal app pool would silently
	// report 0 always, not the real count).
	CountOrganizations(ctx context.Context) (int, error)
}

// PlatformAdminChecker/PlatformAdminSetter are Tenancy's own ports into
// Identity's User - CreateOrganizationService needs "is this caller a
// platform admin" without importing identity/domain directly (this
// codebase's no-cross-context-import rule), same dependency-inversion
// shape as UserReader/RoleReader above. Satisfied structurally by
// identity/adapters/postgres.UserRepository in main.go.
type PlatformAdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

type PlatformAdminSetter interface {
	SetPlatformAdmin(ctx context.Context, userID string, isAdmin bool) error
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
