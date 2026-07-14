// Package domain holds the RBAC context's pure Go types.
// docs/architecture/03-domain-model.md §4 explains why RBAC is modeled
// separately from Identity/Tenancy: every other context asks the same
// "can Subject X do Action Y" question, and that logic needs one home.
package domain

// Permission is a fixed, versioned enum the platform defines - custom
// Roles compose *existing* Permissions, they don't invent new ones
// (docs/architecture/03-domain-model.md §4). Deliberately small right
// now: this walking skeleton only has org-level actions to gate. New
// Permission values get added here as new gate-able actions get built
// (Workspace/Execution's own permissions are a later slice's concern),
// not invented speculatively ahead of an actual endpoint that needs them.
type Permission string

const (
	PermissionOrganizationRead   Permission = "organization:read"
	PermissionOrganizationManage Permission = "organization:manage"
)

// Built-in role names (docs/architecture/03-domain-model.md §4's
// "Owner, Admin, Write, Read - matching the spec's RBAC baseline").
// Owner/Admin and Write/Read are functionally identical for now - there's
// no org-level action yet that distinguishes "can manage" from "is the
// owner," or "can write" from "can only read." That distinction becomes
// real once Workspace/Execution exist (an apply is a write action a
// Write-roled member should get and a Read-roled one shouldn't) - a
// known, deliberately thin spot, not an oversight.
const (
	RoleOwner = "owner"
	RoleAdmin = "admin"
	RoleWrite = "write"
	RoleRead  = "read"
)

// BuiltinRoles is what gets seeded at Control Plane startup
// (docs/architecture/21-deployment.md §4 step 3).
var BuiltinRoles = map[string][]Permission{
	RoleOwner: {PermissionOrganizationRead, PermissionOrganizationManage},
	RoleAdmin: {PermissionOrganizationRead, PermissionOrganizationManage},
	RoleWrite: {PermissionOrganizationRead},
	RoleRead:  {PermissionOrganizationRead},
}

// Role is the RBAC aggregate root - docs/architecture/03-domain-model.md
// §4: organization_id nil means a platform-built-in role.
type Role struct {
	ID              string
	OrganizationID  *string
	Name            string
	Permissions     []Permission
}
