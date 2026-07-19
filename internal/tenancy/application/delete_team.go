package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// subjectTypeTeam is the literal string rbac/domain.SubjectTypeTeam
// itself would use - duplicated here rather than imported (Tenancy
// never imports internal/rbac/domain, dependency-inversion rule per
// docs/architecture/18-backend-structure.md §3), same as how every
// other cross-context literal in this codebase is handled (e.g.
// builtinOwnerRoleName in create_organization.go).
const subjectTypeTeam = "team"

// DeleteTeamInput implements `DELETE /orgs/{org}/teams/{team}` - a real,
// permanent removal (operator-confirmed): the Team, its own
// team_memberships, AND every RoleBinding granted to it - nothing left
// dangling, pointing at a Team that no longer exists.
type DeleteTeamInput struct {
	OrganizationID   string
	RequestingUserID string
	TeamID           string
}

type DeleteTeamService struct {
	repo               TeamRepository
	membership         MembershipRepository
	permChecker        PermissionChecker
	roleBindingCleaner RoleBindingCleaner
}

func NewDeleteTeamService(repo TeamRepository, membership MembershipRepository, permChecker PermissionChecker, roleBindingCleaner RoleBindingCleaner) *DeleteTeamService {
	return &DeleteTeamService{repo: repo, membership: membership, permChecker: permChecker, roleBindingCleaner: roleBindingCleaner}
}

func (s *DeleteTeamService) Execute(ctx context.Context, in DeleteTeamInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	if _, err := s.repo.GetByID(ctx, in.OrganizationID, in.TeamID); err != nil {
		return err
	}

	// RoleBindings cleaned up first - if this step fails, the Team and
	// its memberships still exist, nothing is left orphaned. The
	// reverse order would risk a deleted Team with dangling RoleBindings
	// still pointing at it.
	if err := s.roleBindingCleaner.DeleteForSubject(ctx, in.OrganizationID, subjectTypeTeam, in.TeamID); err != nil {
		return err
	}

	return s.repo.Delete(ctx, in.OrganizationID, in.TeamID)
}
