package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// CreateProjectInput implements `POST /api/v1/orgs/{org}/projects`
// (docs/architecture/04-api-design.md §1). Gated by organization:manage -
// the same permission that gates adding a member (add_member.go):
// creating a Project is an org-structural change, not a day-to-day
// action every "read" member should get, the same reasoning already
// applied there. No new Permission value was introduced for this -
// project:manage doesn't exist yet, deliberately: this is the first
// context where reusing organization:manage instead of minting a
// narrower permission was the honest call, since nothing in this
// codebase yet needs to distinguish "can manage org settings/members"
// from "can create projects."
type CreateProjectInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	Slug             string
	Description      string
}

type CreateProjectService struct {
	repo           ProjectRepository
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
	orgRepo        OrganizationRepository
}

func NewCreateProjectService(repo ProjectRepository, membershipRepo MembershipRepository, permChecker PermissionChecker, orgRepo OrganizationRepository) *CreateProjectService {
	return &CreateProjectService{repo: repo, membershipRepo: membershipRepo, permChecker: permChecker, orgRepo: orgRepo}
}

func (s *CreateProjectService) Execute(ctx context.Context, in CreateProjectInput) (*domain.Project, error) {
	// Membership checked first - see GetOrganizationService's own
	// comment on the "don't reveal existence" reasoning this mirrors: a
	// non-member gets the same 404 a nonexistent org id would, not a 403
	// that would confirm the org is real.
	isMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	// An archived Organization can't grow new structure - the one
	// enforcement point this slice actually wires up (ArchiveOrganization
	// itself only flips a status flag otherwise). Deliberately narrow:
	// this doesn't freeze every possible write against an archived org
	// (Workspace creation, Variables, triggering Runs on *existing*
	// Workspaces all remain unguarded) - a real, named, deferred gap,
	// not an oversight.
	org, err := s.orgRepo.GetByID(ctx, in.OrganizationID)
	if err != nil {
		return nil, err
	}
	if org.Status == domain.OrganizationStatusArchived {
		return nil, domain.ErrOrganizationArchived
	}

	project, err := domain.NewProject(in.OrganizationID, in.Name, in.Slug, in.Description)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, project); err != nil {
		return nil, err
	}

	return project, nil
}
