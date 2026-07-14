package application

import (
	"context"
	"fmt"

	"platform-of-platform/internal/tenancy/domain"
)

var validRoleNames = map[string]bool{"owner": true, "admin": true, "write": true, "read": true}

// ChangeMemberRoleInput implements
// `PUT /api/v1/orgs/{org}/members/{userID}/role` - the endpoint this
// codebase was missing: every RBAC verification so far had to hand-edit
// role_bindings directly in the database to promote a member past the
// default "read" AddMemberService grants. Gated by organization:manage,
// same tier as AddMember.
type ChangeMemberRoleInput struct {
	OrganizationID   string
	RequestingUserID string
	TargetUserID     string
	RoleName         string
}

type ChangeMemberRoleService struct {
	membershipRepo MembershipRepository
	roleChanger    RoleChanger
	permChecker    PermissionChecker
}

func NewChangeMemberRoleService(membershipRepo MembershipRepository, roleChanger RoleChanger, permChecker PermissionChecker) *ChangeMemberRoleService {
	return &ChangeMemberRoleService{membershipRepo: membershipRepo, roleChanger: roleChanger, permChecker: permChecker}
}

func (s *ChangeMemberRoleService) Execute(ctx context.Context, in ChangeMemberRoleInput) error {
	if !validRoleNames[in.RoleName] {
		return &domain.ValidationError{Message: fmt.Sprintf("role %q must be one of owner, admin, write, read", in.RoleName)}
	}

	isRequesterMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isRequesterMember {
		return domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	// The target must actually be a member - changing a non-member's
	// "role" is meaningless (there's no organization-scope binding to
	// replace), and 404 is more honest than silently creating one for
	// someone who was never added via AddMember.
	isTargetMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.TargetUserID)
	if err != nil {
		return err
	}
	if !isTargetMember {
		return domain.ErrOrganizationNotFound
	}

	return s.roleChanger.ReplaceRole(ctx, in.OrganizationID, in.TargetUserID, in.RoleName)
}
