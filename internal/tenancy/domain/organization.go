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

// ErrForbidden is distinct from ErrOrganizationNotFound: it's for an
// authenticated, *known-member* Principal attempting an action their
// role doesn't grant (maps to HTTP 403) - unlike a non-member reading an
// org (404, "don't reveal existence"), a member attempting an action
// they lack permission for already knows the org exists, so there's
// nothing left to hide by 404ing instead.
var ErrForbidden = errors.New("forbidden")

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

const (
	OrganizationStatusActive   = "active"
	OrganizationStatusArchived = "archived"
)

// ErrOrganizationAlreadyArchived - Archive() is not idempotent-as-success
// the way some domain transitions in this codebase are (e.g. a duplicate
// outbox redelivery) - archiving an already-archived org is a genuine
// caller mistake (double DELETE), not a benign no-op, so it's surfaced
// rather than silently swallowed.
var ErrOrganizationAlreadyArchived = errors.New("organization is already archived")

// ErrOrganizationArchived - distinct from ErrForbidden: the requester
// *is* allowed to create a project here, there's just nowhere left to
// put it (same "the action is fine, the resource state isn't" shape as
// execution's own ErrWorkspaceLocked, mapped to 409, not 403/400).
var ErrOrganizationArchived = errors.New("organization is archived")

// ErrOrganizationSlugTaken - organizations.slug is globally unique
// (migrations/0001_init.up.sql) - a duplicate slug surfaces as a real
// Postgres unique violation (OrganizationRepository.Create catches
// pgErr.Code == "23505"), same pattern as RBAC's own
// domain.ErrRoleAlreadyExists, mapped to HTTP 409 at the adapter
// boundary instead of falling through to a generic 500.
var ErrOrganizationSlugTaken = errors.New("an organization with this slug already exists")

// Organization is the Tenancy context's aggregate root - top of the
// containment hierarchy every other aggregate resolves to
// (docs/architecture/03-domain-model.md §2). Status/ArchivedAt implement
// docs/architecture/13-module-identity-rbac-tenancy.md §1's "DELETE
// /orgs/{org} sets status: archived... schedules a background purge job
// 30 days out" - only the soft-delete half is built here (the purge
// reaper is a real, separate, not-yet-built piece, flagged rather than
// silently assumed).
type Organization struct {
	ID         string
	Name       string
	Slug       string
	Settings   map[string]any
	Quota      map[string]any
	Status     string
	ArchivedAt *time.Time
	CreatedAt  time.Time
}

// Archive is the domain-level transition ArchiveOrganizationService
// drives - gated by the caller's own organization:delete permission
// check (RBAC's job, not this method's), this only enforces the
// structural invariant: an org can only be archived once.
func (o *Organization) Archive() error {
	if o.Status == OrganizationStatusArchived {
		return ErrOrganizationAlreadyArchived
	}
	now := time.Now().UTC()
	o.Status = OrganizationStatusArchived
	o.ArchivedAt = &now
	return nil
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
		Status:    OrganizationStatusActive,
		CreatedAt: time.Now().UTC(),
	}, nil
}
