package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrTeamNotFound - same "don't reveal existence to a non-member"
// posture as ErrOrganizationNotFound/ErrProjectNotFound elsewhere in
// this codebase.
var ErrTeamNotFound = errors.New("team not found")

// ErrTeamAlreadyExists - teams.name is unique per organization_id
// (migrations/0012_teams_and_org_archival.up.sql) - same real-Postgres-
// unique-violation-mapped-to-a-sentinel pattern as
// ErrOrganizationSlugTaken/ErrProjectAlreadyExists, mapped to HTTP 409.
var ErrTeamAlreadyExists = errors.New("a team with this name already exists in this organization")

// Team is a group of Users for RBAC binding purposes
// (docs/architecture/03-domain-model.md §2) - deliberately has no
// direct relationship to Project/Workspace; a Team's access is entirely
// mediated through RoleBinding (subject_type='team'), not a structural
// property of Team itself, keeping RBAC the single place access
// decisions are made.
type Team struct {
	ID             string
	OrganizationID string
	Name           string
	CreatedAt      time.Time
}

func NewTeam(organizationID, name string) (*Team, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	return &Team{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Name:           name,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// TeamMembership is an entity, not a full aggregate - same shape/
// reasoning as OrganizationMembership (docs/architecture/03-domain-model.md
// §2): "this user is part of this team," a prerequisite RBAC bindings
// can reference, not itself an authorization grant.
type TeamMembership struct {
	ID             string
	TeamID         string
	OrganizationID string
	UserID         string
	JoinedAt       time.Time
}

func NewTeamMembership(teamID, organizationID, userID string) *TeamMembership {
	return &TeamMembership{
		ID:             uuid.NewString(),
		TeamID:         teamID,
		OrganizationID: organizationID,
		UserID:         userID,
		JoinedAt:       time.Now().UTC(),
	}
}
