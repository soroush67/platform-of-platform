// Package domain holds the RBAC context's pure Go types.
// docs/architecture/03-domain-model.md §4 explains why RBAC is modeled
// separately from Identity/Tenancy: every other context asks the same
// "can Subject X do Action Y" question, and that logic needs one home.
package domain

import (
	"errors"

	"github.com/google/uuid"
)

// Permission is a fixed, versioned enum the platform defines - custom
// Roles compose *existing* Permissions, they don't invent new ones
// (docs/architecture/03-domain-model.md §4). New Permission values get
// added here as new gate-able actions get built, not invented
// speculatively ahead of an actual endpoint that needs them - the
// workspace:* pair below was added exactly when the Workspace context
// gave the first real action worth distinguishing Write from Read over.
type Permission string

const (
	PermissionOrganizationRead   Permission = "organization:read"
	PermissionOrganizationManage Permission = "organization:manage"
	PermissionWorkspaceRead      Permission = "workspace:read"
	PermissionWorkspaceManage    Permission = "workspace:manage"
	// PermissionWorkspaceApply is named directly in
	// docs/architecture/03-domain-model.md §4's own example
	// ("workspace:apply, secret:read, policy_set:manage") - kept
	// distinct from workspace:manage (creating/configuring a Workspace)
	// because triggering or canceling a Run against real infrastructure
	// is a materially different, higher-consequence action than the
	// resource-management one, even though both currently sit at the
	// same Write-role tier.
	PermissionWorkspaceApply Permission = "workspace:apply"
	// PermissionOrganizationDelete is Owner-only (see BuiltinRoles below) -
	// the first real capability that distinguishes Owner from Admin.
	// Gates archiving an Organization (ArchiveOrganizationService),
	// matching docs/architecture/13-module-identity-rbac-tenancy.md §1's
	// "DELETE /orgs/{org} sets status: archived." Billing doesn't exist
	// as a feature in this codebase at all, so it can't be what
	// differentiates the two roles yet - org deletion is the one real,
	// buildable candidate the docs themselves already named.
	PermissionOrganizationDelete Permission = "organization:delete"
)

// AllPermissions is the fixed, versioned enum
// (docs/architecture/03-domain-model.md §4: "a fixed, versioned enum the
// platform defines... custom Roles compose *existing* Permissions, they
// don't invent new ones"). CreateRoleService validates every permission
// in a custom Role's request against this set - not against BuiltinRoles'
// own values, which are a curated subset, not the full enum.
var AllPermissions = map[Permission]bool{
	PermissionOrganizationRead:   true,
	PermissionOrganizationManage: true,
	PermissionOrganizationDelete: true,
	PermissionWorkspaceRead:      true,
	PermissionWorkspaceManage:    true,
	PermissionWorkspaceApply:     true,
}

// Built-in role names (docs/architecture/03-domain-model.md §4's
// "Owner, Admin, Write, Read - matching the spec's RBAC baseline").
const (
	RoleOwner = "owner"
	RoleAdmin = "admin"
	RoleWrite = "write"
	RoleRead  = "read"
)

// BuiltinRoles is what gets seeded (and re-seeded on every startup, via
// an upsert - see the postgres adapter's own comment on why "seed" needs
// to mean "keep in sync," not just "insert once") at Control Plane
// startup (docs/architecture/21-deployment.md §4 step 3).
//
// Owner and Admin now genuinely diverge: only Owner gets
// organization:delete (archiving the Organization -
// ArchiveOrganizationService). Ownership transfer and billing are still
// not modeled at all (no feature exists to gate on billing yet) - this
// is the one real, buildable differentiator the architecture docs
// themselves named, not a full "everything TFC's Owner role can do."
// Write/Read diverge the same way they always have: creating/managing a
// Workspace (workspace:manage) is a day-to-day action a Write-roled
// member gets and a Read-roled one doesn't, while creating a Project or
// Environment stays gated by organization:manage (an org-structural
// decision, deliberately not opened up to Write - see
// create_project.go and create_environment.go's own comments).
var BuiltinRoles = map[string][]Permission{
	RoleOwner: {PermissionOrganizationRead, PermissionOrganizationManage, PermissionOrganizationDelete, PermissionWorkspaceRead, PermissionWorkspaceManage, PermissionWorkspaceApply},
	RoleAdmin: {PermissionOrganizationRead, PermissionOrganizationManage, PermissionWorkspaceRead, PermissionWorkspaceManage, PermissionWorkspaceApply},
	RoleWrite: {PermissionOrganizationRead, PermissionWorkspaceRead, PermissionWorkspaceManage, PermissionWorkspaceApply},
	RoleRead:  {PermissionOrganizationRead, PermissionWorkspaceRead},
}

// Role is the RBAC aggregate root - docs/architecture/03-domain-model.md
// §4: organization_id nil means a platform-built-in role.
type Role struct {
	ID             string
	OrganizationID *string
	Name           string
	Permissions    []Permission
}

// ValidationError distinguishes "the caller sent something invalid"
// (maps to 400) from every other error this context can return - same
// per-context-local type as every other bounded context in this
// codebase (e.g. tenancy/domain.ValidationError), not a shared package,
// per this codebase's own no-cross-context-type-sharing rule.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleAlreadyExists = errors.New("a role with this name already exists in this organization")
	ErrForbidden         = errors.New("forbidden")
)

// NewRole constructs a custom, organization-scoped Role
// (docs/architecture/03-domain-model.md §4: "custom Roles compose
// *existing* Permissions, they don't invent new ones") - every
// permission in the requested set must already be in AllPermissions;
// CreateRoleService is what actually enforces that (this constructor
// just shapes the aggregate, matching every other New* constructor in
// this codebase which validates structural invariants, not
// cross-cutting business rules that belong in the application layer).
func NewRole(organizationID, name string, permissions []Permission) (*Role, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if len(permissions) == 0 {
		return nil, &ValidationError{Message: "permissions must not be empty"}
	}
	return &Role{ID: uuid.NewString(), OrganizationID: &organizationID, Name: name, Permissions: permissions}, nil
}
