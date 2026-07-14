package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// builtinReadRoleName mirrors builtinOwnerRoleName's own reasoning - a
// plain string, not an RBAC domain import.
const builtinReadRoleName = "read"

const permissionOrganizationManage = "organization:manage"

// AddMemberInput - the first real RBAC-gated write in this codebase.
type AddMemberInput struct {
	OrganizationID   string
	RequestingUserID string // the authenticated Principal - must hold organization:manage
	NewMemberUserID  string
}

// AddMemberService implements `POST /api/v1/orgs/{id}/members`. New
// members are granted the built-in "read" role by default - promoting
// someone past that is ChangeMemberRoleService's job
// (change_member_role.go), a real endpoint now, not a deferred gap.
type AddMemberService struct {
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
	roleAssigner   RoleAssigner
}

func NewAddMemberService(membershipRepo MembershipRepository, permChecker PermissionChecker, roleAssigner RoleAssigner) *AddMemberService {
	return &AddMemberService{membershipRepo: membershipRepo, permChecker: permChecker, roleAssigner: roleAssigner}
}

func (s *AddMemberService) Execute(ctx context.Context, in AddMemberInput) error {
	// Membership checked first - a non-member requester gets the same
	// 404 a nonexistent org would give (via the HTTP layer mapping
	// ErrOrganizationNotFound), not a 403 that would confirm the org is
	// real. This used to be a documented gap (both cases flattened to
	// 403); fixed here the same way every other Create*Service in this
	// codebase now orders its checks.
	isMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
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

	membership := domain.NewOrganizationMembership(in.OrganizationID, in.NewMemberUserID)
	if err := s.membershipRepo.Create(ctx, membership); err != nil {
		return err
	}

	return s.roleAssigner.AssignRole(ctx, in.OrganizationID, in.NewMemberUserID, builtinReadRoleName)
}
