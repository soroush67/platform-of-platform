// Package domain holds the Tenancy context's pure Go types - zero imports
// outside the Go stdlib, per docs/architecture/18-backend-structure.md §2.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// ErrOrganizationNotFound is returned by OrganizationRepository.GetByID
// when no row is visible - see the port's own doc comment for why that's
// deliberately ambiguous between "doesn't exist" and "RLS hid it".
var ErrOrganizationNotFound = errors.New("organization not found")

// ValidationError distinguishes "the caller sent something invalid" (maps
// to HTTP 400 at the adapter boundary) from any other error (maps to 500) -
// the domain layer names this distinction, the HTTP adapter is the only
// place that acts on it (docs/architecture/18-backend-structure.md §1's
// shared-error-type note).
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// slugPattern matches docs/architecture/03-domain-model.md §2's "URL-safe"
// requirement: the same shape GitHub/GitLab use for repo slugs (Stage 4
// §1's own reasoning for why slugs, not UUIDs, appear in URLs).
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Organization is the Tenancy context's aggregate root - top of the
// containment hierarchy every other aggregate resolves to
// (docs/architecture/03-domain-model.md §2).
type Organization struct {
	ID        string
	Name      string
	Slug      string
	Settings  map[string]any
	Quota     map[string]any
	CreatedAt time.Time
}

// NewOrganization constructs an Organization, enforcing the invariants a
// caller can't be trusted to have already checked (Stage 3 §2's field
// list). ID/CreatedAt are assigned here, not left to the database default,
// because the adapter needs the ID *before* the INSERT to scope the RLS
// session variable to the row being created (docs/architecture/05-database.md §1).
func NewOrganization(name, slug string) (*Organization, error) {
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if !slugPattern.MatchString(slug) {
		return nil, &ValidationError{Message: fmt.Sprintf("slug %q must be lowercase alphanumeric with hyphens", slug)}
	}

	return &Organization{
		ID:        uuid.NewString(),
		Name:      name,
		Slug:      slug,
		Settings:  map[string]any{},
		Quota:     map[string]any{},
		CreatedAt: time.Now().UTC(),
	}, nil
}
