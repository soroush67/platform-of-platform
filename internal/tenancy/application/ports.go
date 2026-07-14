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
}
