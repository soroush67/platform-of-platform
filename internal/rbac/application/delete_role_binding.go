package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

// DeleteRoleBindingInput implements `DELETE /orgs/{org}/role-bindings/{id}` -
// a real, permanent removal (operator-confirmed, not a soft/client-side
// hide) - same organization:manage gate CreateRoleBindingService already
// uses, for the same reason: creating and removing an access grant are
// the same tier of consequence.
type DeleteRoleBindingInput struct {
	OrganizationID   string
	RequestingUserID string
	BindingID        string
}

type DeleteRoleBindingService struct {
	repo        RoleBindingRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDeleteRoleBindingService(repo RoleBindingRepository, membership MembershipChecker, permChecker PermissionChecker) *DeleteRoleBindingService {
	return &DeleteRoleBindingService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *DeleteRoleBindingService) Execute(ctx context.Context, in DeleteRoleBindingInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	return s.repo.Delete(ctx, in.OrganizationID, in.BindingID)
}
