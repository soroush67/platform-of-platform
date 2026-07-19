package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// TeamRepository is Tenancy's own port for the Team aggregate
// (docs/architecture/03-domain-model.md §2) - same shape/reasoning as
// ProjectRepository.
type TeamRepository interface {
	Create(ctx context.Context, team *domain.Team) error
	AddMember(ctx context.Context, membership *domain.TeamMembership) error
	RemoveMember(ctx context.Context, organizationID, teamID, userID string) error
	GetByID(ctx context.Context, organizationID, teamID string) (*domain.Team, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Team, error)
	// ListMembers backs ListTeamMembersService's own new roster endpoint
	// (list_team_members.go).
	ListMembers(ctx context.Context, organizationID, teamID string) ([]*domain.TeamMembership, error)
	// Update backs UpdateTeamService (rename). Delete backs
	// DeleteTeamService - removes the Team's own team_memberships too,
	// in the same transaction (see the postgres adapter's own comment on
	// why that's required, not optional).
	Update(ctx context.Context, team *domain.Team) error
	Delete(ctx context.Context, organizationID, teamID string) error
}

// CreateTeamInput implements `POST /orgs/{org}/teams`
// (docs/architecture/13-module-identity-rbac-tenancy.md §1). Gated by
// organization:manage - same permission as adding a member/creating a
// project, the established "org-structural action" tier.
type CreateTeamInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
}

type CreateTeamService struct {
	repo        TeamRepository
	membership  MembershipRepository
	permChecker PermissionChecker
}

func NewCreateTeamService(repo TeamRepository, membership MembershipRepository, permChecker PermissionChecker) *CreateTeamService {
	return &CreateTeamService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *CreateTeamService) Execute(ctx context.Context, in CreateTeamInput) (*domain.Team, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
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

	team, err := domain.NewTeam(in.OrganizationID, in.Name)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, team); err != nil {
		return nil, err
	}

	return team, nil
}
