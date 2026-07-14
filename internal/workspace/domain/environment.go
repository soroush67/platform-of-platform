// Package domain holds the Workspace & Environment context's pure Go
// types (docs/architecture/03-domain-model.md §5) - "the hub the product
// actually revolves around" per the Stage 3 context map, its own bounded
// context, not folded into Tenancy even though every row here resolves
// to a Project.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var ErrEnvironmentNotFound = errors.New("environment not found")

// ErrProjectNotFound - this context's own sentinel for "the project_id
// in the URL doesn't genuinely belong to this org" (checked via
// ProjectChecker, application/ports.go), distinct from
// tenancy/domain.ErrProjectNotFound: Workspace can't import Tenancy's
// domain package at all (docs/architecture/18-backend-structure.md §3).
var ErrProjectNotFound = errors.New("project not found")

// ErrForbidden - same "known member, action their role doesn't grant"
// meaning as tenancy/domain.ErrForbidden, redeclared here for the same
// cross-context-import reason as ErrProjectNotFound above.
var ErrForbidden = errors.New("forbidden")

// ErrOrganizationArchived - same meaning as tenancy/domain's own sentinel
// (maps to 409, "the action is fine, there's nowhere left to put it"),
// redeclared here for the same cross-context-import reason as
// ErrProjectNotFound above. CreateWorkspaceService checks this before
// creating a new Workspace in an archived Organization.
var ErrOrganizationArchived = errors.New("organization is archived")

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// namePattern - Environment/Workspace names aren't URL slugs (they're
// not top-level resources addressed by slug the way Organization/Project
// are, per docs/architecture/04-api-design.md §1's resource-path list -
// they sit under a UUID-addressed project path) but still need *some*
// shape constraint so a blank or whitespace-only name isn't accepted.
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Environment is an aggregate root referencing ProjectID
// (docs/architecture/03-domain-model.md §5) - "what carries [promotion]
// sequencing plus environment-scoped Variables that cascade into every
// Workspace inside it." Variables don't exist yet in this codebase
// (Stage 8's own context, not built here) - PromotionRank/RequiresApproval
// are the two fields this slice actually has a use for.
type Environment struct {
	ID               string
	OrganizationID   string
	ProjectID        string
	Name             string
	PromotionRank    int
	RequiresApproval bool
	CreatedAt        time.Time
}

func NewEnvironment(organizationID, projectID, name string, promotionRank int, requiresApproval bool) (*Environment, error) {
	if organizationID == "" || projectID == "" {
		return nil, &ValidationError{Message: "organization_id and project_id are required"}
	}
	if !namePattern.MatchString(name) {
		return nil, &ValidationError{Message: fmt.Sprintf("name %q must start with a letter/digit and contain only letters, digits, - or _", name)}
	}

	return &Environment{
		ID:               uuid.NewString(),
		OrganizationID:   organizationID,
		ProjectID:        projectID,
		Name:             name,
		PromotionRank:    promotionRank,
		RequiresApproval: requiresApproval,
		CreatedAt:        time.Now().UTC(),
	}, nil
}
