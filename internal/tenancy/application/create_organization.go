package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// CreateOrganizationInput is the use case's own request shape - the HTTP
// adapter maps its request DTO onto this, the domain never sees the wire
// format directly (docs/architecture/18-backend-structure.md §2).
type CreateOrganizationInput struct {
	Name string
	Slug string
}

// CreateOrganizationService implements the `POST /api/v1/orgs` use case
// (docs/architecture/04-api-design.md §1). Deliberately unauthenticated at
// this stage - the walking skeleton doesn't have Identity/RBAC's auth
// middleware wired yet; that's the next slice, not silently skipped here.
type CreateOrganizationService struct {
	repo OrganizationRepository
}

func NewCreateOrganizationService(repo OrganizationRepository) *CreateOrganizationService {
	return &CreateOrganizationService{repo: repo}
}

func (s *CreateOrganizationService) Execute(ctx context.Context, in CreateOrganizationInput) (*domain.Organization, error) {
	org, err := domain.NewOrganization(in.Name, in.Slug)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, org); err != nil {
		return nil, err
	}

	return org, nil
}
