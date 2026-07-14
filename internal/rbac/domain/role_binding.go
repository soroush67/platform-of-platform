package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	SubjectTypeUser           = "user"
	SubjectTypeTeam           = "team"
	SubjectTypeServiceAccount = "service_account"

	ScopeTypeOrganization = "organization"
	ScopeTypeProject      = "project"
	ScopeTypeWorkspace    = "workspace"
)

// ValidSubjectTypes/ValidScopeTypes - what CreateRoleBindingService
// actually accepts today. The schema's own CHECK constraints
// (migrations/0001_init.up.sql) allow 'service_account' as a
// subject_type too, but no ServiceAccount aggregate exists yet in this
// codebase (Identity context gap, not RBAC's) - rejecting it here at
// the application layer, not the database, keeps that gap honest rather
// than accepting bindings for subjects that can never actually exist.
var ValidSubjectTypes = map[string]bool{SubjectTypeUser: true, SubjectTypeTeam: true}
var ValidScopeTypes = map[string]bool{ScopeTypeOrganization: true, ScopeTypeProject: true, ScopeTypeWorkspace: true}

// RoleBinding is the actual grant: "Role R applies to Subject S at Scope
// T" (docs/architecture/03-domain-model.md §4). User and Team subjects,
// Organization/Project/Workspace scopes are all real now - see
// CreateRoleBindingService for the validation that keeps a binding's
// scope_id honestly pointed at a resource that exists in this same
// Organization.
type RoleBinding struct {
	ID             string
	OrganizationID string
	RoleID         string
	SubjectType    string
	SubjectID      string
	ScopeType      string
	ScopeID        string
	CreatedAt      time.Time
}

// NewRoleBinding is the generic constructor CreateRoleBindingService uses
// for the real POST /role-bindings endpoint - unlike
// NewOrganizationScopeBinding below (kept for the built-in-role
// bootstrap/replace paths, which are always user+organization), this
// accepts any of the subject/scope combinations ValidSubjectTypes/
// ValidScopeTypes now allow.
func NewRoleBinding(organizationID, roleID, subjectType, subjectID, scopeType, scopeID string) *RoleBinding {
	return &RoleBinding{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		RoleID:         roleID,
		SubjectType:    subjectType,
		SubjectID:      subjectID,
		ScopeType:      scopeType,
		ScopeID:        scopeID,
		CreatedAt:      time.Now().UTC(),
	}
}

// NewOrganizationScopeBinding constructs a binding at organization scope
// for any role - despite this function's former name
// (NewOrganizationOwnerBinding), it was already used generically for
// every built-in role (AssignRole's own "read" default in
// add_member.go, not just "owner"); renamed to describe what it
// actually does, not what its first caller happened to be for.
func NewOrganizationScopeBinding(organizationID, roleID, userID string) *RoleBinding {
	return &RoleBinding{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		RoleID:         roleID,
		SubjectType:    SubjectTypeUser,
		SubjectID:      userID,
		ScopeType:      ScopeTypeOrganization,
		ScopeID:        organizationID,
		CreatedAt:      time.Now().UTC(),
	}
}
