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

	// EffectAllow/EffectDeny (migrations/0016_role_binding_effect.up.sql) -
	// AWS-IAM-style evaluation, not Kubernetes RBAC's pure-additive-only
	// model: HasPermissionAtScope treats any matching Deny as an
	// unconditional override of every matching Allow, regardless of
	// which scope each came from. This is what actually implements
	// docs/architecture/03-domain-model.md §4's "a binding at a higher
	// scope implies the grant... unless a more specific binding narrows
	// it" - an org-wide Allow can now genuinely be narrowed by a single
	// project- or workspace-scope Deny.
	EffectAllow = "allow"
	EffectDeny  = "deny"
)

// ValidSubjectTypes/ValidScopeTypes/ValidEffects - what
// CreateRoleBindingService actually accepts. ServiceAccount is now real
// (internal/identity/domain/service_account.go) - previously rejected
// here specifically because no ServiceAccount aggregate existed yet to
// be a genuine subject.
var ValidSubjectTypes = map[string]bool{SubjectTypeUser: true, SubjectTypeTeam: true, SubjectTypeServiceAccount: true}
var ValidScopeTypes = map[string]bool{ScopeTypeOrganization: true, ScopeTypeProject: true, ScopeTypeWorkspace: true}
var ValidEffects = map[string]bool{EffectAllow: true, EffectDeny: true}

// RoleBinding is the actual grant (or denial - see EffectDeny above):
// "Role R applies to Subject S at Scope T" (docs/architecture/
// 03-domain-model.md §4). User/Team/ServiceAccount subjects,
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
	Effect         string
	CreatedAt      time.Time
}

// NewRoleBinding is the generic constructor CreateRoleBindingService uses
// for the real POST /role-bindings endpoint - unlike
// NewOrganizationScopeBinding below (kept for the built-in-role
// bootstrap/replace paths, which are always user+organization+allow),
// this accepts any of the subject/scope/effect combinations
// ValidSubjectTypes/ValidScopeTypes/ValidEffects now allow.
func NewRoleBinding(organizationID, roleID, subjectType, subjectID, scopeType, scopeID, effect string) *RoleBinding {
	return &RoleBinding{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		RoleID:         roleID,
		SubjectType:    subjectType,
		SubjectID:      subjectID,
		ScopeType:      scopeType,
		ScopeID:        scopeID,
		Effect:         effect,
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
		Effect:         EffectAllow,
		CreatedAt:      time.Now().UTC(),
	}
}
