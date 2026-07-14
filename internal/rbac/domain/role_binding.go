package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	SubjectTypeUser = "user"

	ScopeTypeOrganization = "organization"
)

// RoleBinding is the actual grant: "Role R applies to Subject S at Scope
// T" (docs/architecture/03-domain-model.md §4). Only user-subject,
// organization-scope bindings exist so far - Team doesn't exist yet
// (no Team aggregate built), and Project/Workspace scopes have nothing
// to scope to yet either. Both are real, later gaps, not modeled away.
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
