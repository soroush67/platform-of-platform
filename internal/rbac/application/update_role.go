package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

// UpdateRoleInput implements `PUT /orgs/{org}/roles/{role}` - operator-
// confirmed behavior: this updates the Role in place (name stays fixed,
// only the permission set changes), so every existing RoleBinding that
// already references this Role picks up the new permissions immediately -
// the same way every built-in Role's own permission set already applies
// to every binding pointing at it, not a versioned/copy-on-write role.
type UpdateRoleInput struct {
	OrganizationID   string
	RequestingUserID string
	RoleID           string
	Permissions      []string
}

type UpdateRoleService struct {
	repo        RoleRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewUpdateRoleService(repo RoleRepository, membership MembershipChecker, permChecker PermissionChecker) *UpdateRoleService {
	return &UpdateRoleService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *UpdateRoleService) Execute(ctx context.Context, in UpdateRoleInput) (*domain.Role, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	role, err := s.repo.GetByID(ctx, in.OrganizationID, in.RoleID)
	if err != nil {
		return nil, err
	}
	// Built-in roles (organization_id nil) stay uneditable - same
	// "modeled fully, changeable only where it's genuinely this org's
	// own thing" posture as every other builtin-vs-custom split in this
	// codebase (e.g. builtin Roles were already never DELETE-able).
	if role.OrganizationID == nil {
		return nil, domain.ErrForbidden
	}

	// Same "custom Roles compose *existing* Permissions" validation
	// CreateRoleService already enforces.
	permissions := make([]domain.Permission, 0, len(in.Permissions))
	for _, p := range in.Permissions {
		permission := domain.Permission(p)
		if !domain.AllPermissions[permission] {
			return nil, &domain.ValidationError{Message: "unknown permission: " + p}
		}
		permissions = append(permissions, permission)
	}
	if len(permissions) == 0 {
		return nil, &domain.ValidationError{Message: "permissions must not be empty"}
	}

	role.Permissions = permissions
	if err := s.repo.Update(ctx, role); err != nil {
		return nil, err
	}

	return role, nil
}
