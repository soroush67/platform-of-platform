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
	// CreatedByUserID is the authenticated Principal creating this org
	// (docs/architecture/04-api-design.md §4) - required now that this
	// endpoint sits behind RequireAuth. The creator becomes the org's
	// first OrganizationMembership automatically: the real-world flow
	// this mirrors (create an org, you're in it) doubles as this
	// walking skeleton's only way to bootstrap membership at all, since
	// there's no invite/add-member endpoint yet.
	CreatedByUserID string
}

// CreateOrganizationService implements the `POST /api/v1/orgs` use case
// (docs/architecture/04-api-design.md §1).
type CreateOrganizationService struct {
	orgRepo        OrganizationRepository
	membershipRepo MembershipRepository
}

func NewCreateOrganizationService(orgRepo OrganizationRepository, membershipRepo MembershipRepository) *CreateOrganizationService {
	return &CreateOrganizationService{orgRepo: orgRepo, membershipRepo: membershipRepo}
}

func (s *CreateOrganizationService) Execute(ctx context.Context, in CreateOrganizationInput) (*domain.Organization, error) {
	org, err := domain.NewOrganization(in.Name, in.Slug)
	if err != nil {
		return nil, err
	}

	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, err
	}

	membership := domain.NewOrganizationMembership(org.ID, in.CreatedByUserID)
	if err := s.membershipRepo.Create(ctx, membership); err != nil {
		return nil, err
	}

	return org, nil
}
