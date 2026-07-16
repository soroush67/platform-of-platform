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
// (docs/architecture/04-api-design.md §1). Gated by platform-admin
// status - Organization creation is no longer self-service (previously
// any authenticated user), matching the operator's own "like a top-level
// Group in GitLab" framing. The one exception is the platform's very
// first Organization ever: mirrors RequireAuthOrFirstUserBootstrap's own
// "count==0 opens the door" bootstrap shape (internal/platform/
// httpserver/auth_middleware.go), but at the application layer rather
// than the HTTP layer, since by the time anyone can create an
// Organization they're already a real authenticated user - that
// bootstrap problem is already solved separately. The creator of the
// first-ever Organization is granted platform-admin as a side effect,
// so there's always at least one real admin able to create the next
// one (and to promote others - see identity/application/
// set_platform_admin.go).
//
// Known, accepted narrow race, same one RequireAuthOrFirstUserBootstrap's
// own doc comment already accepts: two concurrent requests before the
// first Organization commits could both see count==0 and both succeed -
// not worth guarding, a one-time boot sequence, not a standing attack
// surface.
type CreateOrganizationService struct {
	orgRepo          OrganizationRepository
	membershipRepo   MembershipRepository
	roleAssigner     RoleAssigner
	rootMembership   RootMembershipRepository
	platformAdmins   PlatformAdminChecker
	platformAdminSet PlatformAdminSetter
}

func NewCreateOrganizationService(orgRepo OrganizationRepository, membershipRepo MembershipRepository, roleAssigner RoleAssigner, rootMembership RootMembershipRepository, platformAdmins PlatformAdminChecker, platformAdminSet PlatformAdminSetter) *CreateOrganizationService {
	return &CreateOrganizationService{
		orgRepo: orgRepo, membershipRepo: membershipRepo, roleAssigner: roleAssigner,
		rootMembership: rootMembership, platformAdmins: platformAdmins, platformAdminSet: platformAdminSet,
	}
}

func (s *CreateOrganizationService) Execute(ctx context.Context, in CreateOrganizationInput) (*domain.Organization, error) {
	count, err := s.rootMembership.CountOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	isFirstOrgEver := count == 0

	if !isFirstOrgEver {
		isAdmin, err := s.platformAdmins.IsPlatformAdmin(ctx, in.CreatedByUserID)
		if err != nil {
			return nil, err
		}
		if !isAdmin {
			return nil, domain.ErrForbidden
		}
	}

	org, err := domain.NewOrganization(in.Name, in.Slug)
	if err != nil {
		return nil, err
	}

	if err := s.orgRepo.Create(ctx, org, in.CreatedByUserID); err != nil {
		return nil, err
	}

	membership := domain.NewOrganizationMembership(org.ID, in.CreatedByUserID)
	if err := s.membershipRepo.Create(ctx, membership); err != nil {
		return nil, err
	}

	if err := s.roleAssigner.AssignRole(ctx, org.ID, in.CreatedByUserID, builtinOwnerRoleName); err != nil {
		return nil, err
	}

	if isFirstOrgEver {
		if err := s.platformAdminSet.SetPlatformAdmin(ctx, in.CreatedByUserID, true); err != nil {
			return nil, err
		}
	}

	return org, nil
}
