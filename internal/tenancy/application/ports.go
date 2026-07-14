package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// OrganizationRepository is the port the /adapters/postgres package
// satisfies - this package declares the interface shaped for its own
// use, per the dependency-inversion rule in
// docs/architecture/18-backend-structure.md §3.
type OrganizationRepository interface {
	Create(ctx context.Context, org *domain.Organization) error
	// GetByID returns ErrOrganizationNotFound if no row is visible for id -
	// either because it genuinely doesn't exist, or because RLS hid it
	// (the two are indistinguishable by design, per
	// docs/architecture/05-database.md §1 - a 404 here reveals nothing
	// about whether some *other* org's id happens to exist).
	GetByID(ctx context.Context, id string) (*domain.Organization, error)
}
