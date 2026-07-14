package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// GetOrganizationService implements `GET /api/v1/orgs/{id}`
// (docs/architecture/04-api-design.md §1). Now requires the
// authenticated Principal's user id and checks OrganizationMembership
// before ever calling GetByID - closing the gap the previous version of
// this file documented: organizations' own RLS is self-referential
// (scoping app.current_org_id to the very id in the URL trivially
// satisfies it for anyone), so the membership check, not the
// organizations table's RLS, is what actually makes this cross-tenant
// safe now.
type GetOrganizationService struct {
	orgRepo        OrganizationRepository
	membershipRepo MembershipRepository
}

func NewGetOrganizationService(orgRepo OrganizationRepository, membershipRepo MembershipRepository) *GetOrganizationService {
	return &GetOrganizationService{orgRepo: orgRepo, membershipRepo: membershipRepo}
}

func (s *GetOrganizationService) Execute(ctx context.Context, id, requestingUserID string) (*domain.Organization, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, id, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		// Same "don't reveal existence" posture as domain.ErrOrganizationNotFound
		// itself - a non-member gets exactly the response they'd get for
		// an id that doesn't exist at all, not a 403 that would confirm
		// the org is real.
		return nil, domain.ErrOrganizationNotFound
	}

	return s.orgRepo.GetByID(ctx, id)
}
