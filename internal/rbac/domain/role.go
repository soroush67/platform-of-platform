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
	PermissionWorkspaceApply  Permission = "workspace:apply"
	PermissionWorkspaceDelete Permission = "workspace:delete"
	// PermissionOrganizationDelete is Owner-only (see BuiltinRoles below) -
	// the first real capability that distinguishes Owner from Admin.
	// Gates archiving an Organization (ArchiveOrganizationService),
	// matching docs/architecture/13-module-identity-rbac-tenancy.md §1's
	// "DELETE /orgs/{org} sets status: archived." Billing doesn't exist
	// as a feature in this codebase at all, so it can't be what
	// differentiates the two roles yet - org deletion is the one real,
	// buildable candidate the docs themselves already named.
	PermissionOrganizationDelete Permission = "organization:delete"
	// PermissionProjectRead/Manage gate Tenancy's Project resource - split
	// out from organization:manage (which create_project.go used to
	// reuse) so a Role can grant "create/manage Projects" independently
	// of every other org-structural action, matching the same
	// one-permission-per-menu split Fleet's own permissions below use.
	PermissionProjectRead   Permission = "project:read"
	PermissionProjectManage Permission = "project:manage"
	// PermissionProjectDelete/WorkspaceDelete/MachineDelete/
	// NetworkVolumeDelete/ComposeFileDelete each extend the same
	// stricter-than-manage narrowing organization:delete already
	// established to every other resource that now has a real delete
	// action - Owner-only (see BuiltinRoles below), so Admin's existing
	// full *:manage parity with Owner stops short of destructive-delete
	// the same way it already stops short of organization:delete.
	PermissionProjectDelete Permission = "project:delete"
	// PermissionMachineRead/Manage, PermissionNetworkVolumeRead/Manage,
	// PermissionComposeFileRead/Manage, PermissionOperationRead/Deploy
	// each gate exactly one Fleet (internal/fleet - the ported
	// compose-platform functionality) nav menu independently - replaces
	// the earlier fleet:read/manage/deploy, which bundled all of
	// Machines/Networks/Volumes/ComposeFiles/Operations behind one
	// shared permission and made it impossible to grant a Role "manage
	// Machines" without also granting "manage Operations." Deploy is
	// kept distinct from Manage for the same reason workspace:apply is
	// distinct from workspace:manage - triggering a real SSH deploy
	// against a real remote machine is a materially higher-consequence
	// action than managing the catalog resources themselves.
	PermissionMachineRead   Permission = "machine:read"
	PermissionMachineManage Permission = "machine:manage"
	PermissionMachineDelete Permission = "machine:delete"
	// PermissionNetworkVolumeRead/Manage gates BOTH the Network and
	// Volume catalogs as one pair - "Networks & volumes" is a single
	// combined nav menu/page, not two, so it gets one permission pair
	// like every other menu does.
	PermissionNetworkVolumeRead   Permission = "network_volume:read"
	PermissionNetworkVolumeManage Permission = "network_volume:manage"
	PermissionNetworkVolumeDelete Permission = "network_volume:delete"
	// PermissionComposeFileRead/Manage also gates fleet_variables CRUD
	// and attaching/detaching a Network or Volume TO a ComposeFile
	// (AttachmentService, variable.go) - both of those are only ever
	// reached from ComposeFileDetailPage, itself only reachable via the
	// Compose files menu.
	PermissionComposeFileRead   Permission = "compose_file:read"
	PermissionComposeFileManage Permission = "compose_file:manage"
	PermissionComposeFileDelete Permission = "compose_file:delete"
	// PermissionOperationRead/Deploy - Operations has no separate
	// "manage" tier: the only mutating action on this menu is triggering
	// a real SSH deploy, so Deploy is its one write-tier permission.
	PermissionOperationRead   Permission = "operation:read"
	PermissionOperationDeploy Permission = "operation:deploy"
)

// AllPermissions is the fixed, versioned enum
// (docs/architecture/03-domain-model.md §4: "a fixed, versioned enum the
// platform defines... custom Roles compose *existing* Permissions, they
// don't invent new ones"). CreateRoleService validates every permission
// in a custom Role's request against this set - not against BuiltinRoles'
// own values, which are a curated subset, not the full enum.
var AllPermissions = map[Permission]bool{
	PermissionOrganizationRead:    true,
	PermissionOrganizationManage:  true,
	PermissionOrganizationDelete:  true,
	PermissionProjectRead:         true,
	PermissionProjectManage:       true,
	PermissionProjectDelete:       true,
	PermissionWorkspaceRead:       true,
	PermissionWorkspaceManage:     true,
	PermissionWorkspaceApply:      true,
	PermissionWorkspaceDelete:     true,
	PermissionMachineRead:         true,
	PermissionMachineManage:       true,
	PermissionMachineDelete:       true,
	PermissionNetworkVolumeRead:   true,
	PermissionNetworkVolumeManage: true,
	PermissionNetworkVolumeDelete: true,
	PermissionComposeFileRead:     true,
	PermissionComposeFileManage:   true,
	PermissionComposeFileDelete:   true,
	PermissionOperationRead:       true,
	PermissionOperationDeploy:     true,
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
// ArchiveOrganizationService) and, extending that same narrowing to
// every resource that now has a real delete action, project:delete/
// workspace:delete/machine:delete/network_volume:delete/
// compose_file:delete - Admin keeps full parity with Owner on every
// *:manage permission (create/configure/update), but destructive delete
// stays Owner-only across the board, not just for Organizations.
// Ownership transfer and billing are still not modeled at all (no
// feature exists to gate on billing yet) - this is the one real,
// buildable differentiator the architecture docs themselves named, not
// a full "everything TFC's Owner role can do."
// Write/Read diverge the same way they always have: creating/managing a
// Workspace (workspace:manage) is a day-to-day action a Write-roled
// member gets and a Read-roled one doesn't. project:manage is
// deliberately NOT given to Write either (an org-structural decision,
// same as before this permission had its own name - see
// create_project.go's own comment) - every other new per-menu
// permission below (machine/network_volume/compose_file/operation)
// keeps the tier Fleet's own permissions already had (Write gets
// manage/deploy), unchanged from before the fleet:* split.
var BuiltinRoles = map[string][]Permission{
	RoleOwner: {
		PermissionOrganizationRead, PermissionOrganizationManage, PermissionOrganizationDelete,
		PermissionProjectRead, PermissionProjectManage, PermissionProjectDelete,
		PermissionWorkspaceRead, PermissionWorkspaceManage, PermissionWorkspaceApply, PermissionWorkspaceDelete,
		PermissionMachineRead, PermissionMachineManage, PermissionMachineDelete,
		PermissionNetworkVolumeRead, PermissionNetworkVolumeManage, PermissionNetworkVolumeDelete,
		PermissionComposeFileRead, PermissionComposeFileManage, PermissionComposeFileDelete,
		PermissionOperationRead, PermissionOperationDeploy,
	},
	RoleAdmin: {
		PermissionOrganizationRead, PermissionOrganizationManage,
		PermissionProjectRead, PermissionProjectManage,
		PermissionWorkspaceRead, PermissionWorkspaceManage, PermissionWorkspaceApply,
		PermissionMachineRead, PermissionMachineManage,
		PermissionNetworkVolumeRead, PermissionNetworkVolumeManage,
		PermissionComposeFileRead, PermissionComposeFileManage,
		PermissionOperationRead, PermissionOperationDeploy,
	},
	RoleWrite: {
		PermissionOrganizationRead,
		PermissionProjectRead,
		PermissionWorkspaceRead, PermissionWorkspaceManage, PermissionWorkspaceApply,
		PermissionMachineRead, PermissionMachineManage,
		PermissionNetworkVolumeRead, PermissionNetworkVolumeManage,
		PermissionComposeFileRead, PermissionComposeFileManage,
		PermissionOperationRead, PermissionOperationDeploy,
	},
	RoleRead: {
		PermissionOrganizationRead,
		PermissionProjectRead,
		PermissionWorkspaceRead,
		PermissionMachineRead,
		PermissionNetworkVolumeRead,
		PermissionComposeFileRead,
		PermissionOperationRead,
	},
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
