// Package domain holds the RBAC context's pure Go types.
// docs/architecture/03-domain-model.md §4 explains why RBAC is modeled
// separately from Identity/Tenancy: every other context asks the same
// "can Subject X do Action Y" question, and that logic needs one home.
package domain

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
)

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
// Owner/Admin are still functionally identical - nothing yet
// distinguishes "is the owner" from "can manage," that's still a real,
// deferred gap (billing, ownership transfer - Stage 13 territory). But
// Write/Read are no longer identical: creating/managing a Workspace
// (workspace:manage) is a day-to-day action a Write-roled member gets
// and a Read-roled one doesn't, while creating a Project or Environment
// stays gated by organization:manage (an org-structural decision,
// deliberately not opened up to Write - see create_project.go and
// create_environment.go's own comments).
var BuiltinRoles = map[string][]Permission{
	RoleOwner: {PermissionOrganizationRead, PermissionOrganizationManage, PermissionWorkspaceRead, PermissionWorkspaceManage},
	RoleAdmin: {PermissionOrganizationRead, PermissionOrganizationManage, PermissionWorkspaceRead, PermissionWorkspaceManage},
	RoleWrite: {PermissionOrganizationRead, PermissionWorkspaceRead, PermissionWorkspaceManage},
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
