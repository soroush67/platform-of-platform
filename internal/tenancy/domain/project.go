package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrProjectNotFound - same "doesn't exist or RLS/membership hid it"
// ambiguity as ErrOrganizationNotFound's own doc comment.
var ErrProjectNotFound = errors.New("project not found")

// Project is an aggregate root referencing organization_id
// (docs/architecture/03-domain-model.md §2) - "a grouping of
// Environments/Workspaces - typically 'one product/service' inside an
// org." Its own invariant: OrganizationID is immutable after creation
// (§2's "a project cannot move between organizations") - there's no
// UpdateOrganization method on this type, by omission, not oversight.
type Project struct {
	ID             string
	OrganizationID string
	Name           string
	Slug           string
	Description    string
	CreatedAt      time.Time
}

// NewProject reuses slugPattern from organization.go - same package,
// same "URL-safe" requirement (docs/architecture/03-domain-model.md §2's
// slug uniqueness is scoped *within* an org, unlike Organization.Slug's
// global uniqueness, but the shape validation is identical).
func NewProject(organizationID, name, slug, description string) (*Project, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if !slugPattern.MatchString(slug) {
		return nil, &ValidationError{Message: fmt.Sprintf("slug %q must be lowercase alphanumeric with hyphens", slug)}
	}

	return &Project{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		Name:           name,
		Slug:           slug,
		Description:    description,
		CreatedAt:      time.Now().UTC(),
	}, nil
}
