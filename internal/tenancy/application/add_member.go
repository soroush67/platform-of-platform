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
// someone to write/admin/owner is a real, later feature (a role-change
// endpoint), not built here; this only proves the permission-check path
// end to end for the one write action this walking skeleton actually has.
//
// Known simplification: HasPermission returning false doesn't distinguish
// "you're a member but your role lacks organization:manage" from "you
// aren't a member of this org at all" - both map to a flat 403 below,
// unlike GetOrganizationService's 404-for-non-members. A stricter version
// would 404 the second case too (same "don't reveal existence" reasoning),
// deferred here rather than built speculatively.
type AddMemberService struct {
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
	roleAssigner   RoleAssigner
}

func NewAddMemberService(membershipRepo MembershipRepository, permChecker PermissionChecker, roleAssigner RoleAssigner) *AddMemberService {
	return &AddMemberService{membershipRepo: membershipRepo, permChecker: permChecker, roleAssigner: roleAssigner}
}

func (s *AddMemberService) Execute(ctx context.Context, in AddMemberInput) error {
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
