package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// builtinOwnerRoleName is a plain string, not an import of
// internal/rbac/domain.RoleOwner - Tenancy depends on RBAC only through
// the RoleAssigner port it declares itself (docs/architecture/18-backend-
// structure.md §3), never on RBAC's own domain types.
const builtinOwnerRoleName = "owner"

// CreateOrganizationInput is the use case's own request shape - the HTTP
// adapter maps its request DTO onto this, the domain never sees the wire
// format directly (docs/architecture/18-backend-structure.md §2).
type CreateOrganizationInput struct {
	Name string
	Slug string
	// CreatedByUserID is the authenticated Principal creating this org
	// (docs/architecture/04-api-design.md §4) - required now that this
	// endpoint sits behind RequireAuth. The creator becomes the org's
	// first OrganizationMembership *and* is granted the built-in "owner"
	// role at organization scope: the real-world flow this mirrors
	// (create an org, you're in it, you own it) doubles as this walking
	// skeleton's only way to bootstrap RBAC bindings at all, since
	// there's no invite/assign-role endpoint yet beyond membership.
	CreatedByUserID string
}

// CreateOrganizationService implements the `POST /api/v1/orgs` use case
// (docs/architecture/04-api-design.md §1).
type CreateOrganizationService struct {
	orgRepo        OrganizationRepository
	membershipRepo MembershipRepository
	roleAssigner   RoleAssigner
}

func NewCreateOrganizationService(orgRepo OrganizationRepository, membershipRepo MembershipRepository, roleAssigner RoleAssigner) *CreateOrganizationService {
	return &CreateOrganizationService{orgRepo: orgRepo, membershipRepo: membershipRepo, roleAssigner: roleAssigner}
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

	if err := s.roleAssigner.AssignRole(ctx, org.ID, in.CreatedByUserID, builtinOwnerRoleName); err != nil {
		return nil, err
	}

	return org, nil
}
