package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

const permissionOrganizationManage = "organization:manage"

// CreateRoleInput implements `POST /orgs/{org}/roles`
// (docs/architecture/13-module-identity-rbac-tenancy.md §3). Gated by
// organization:manage - the same permission that gates every other
// org-structural action (add members, create projects) - Admin can
// define custom roles, matching "Admin: every Permission except
// billing/org-deletion."
type CreateRoleInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	Permissions      []string
}

type CreateRoleService struct {
	repo        RoleRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewCreateRoleService(repo RoleRepository, membership MembershipChecker, permChecker PermissionChecker) *CreateRoleService {
	return &CreateRoleService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *CreateRoleService) Execute(ctx context.Context, in CreateRoleInput) (*domain.Role, error) {
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

	// docs/architecture/03-domain-model.md §4: "custom Roles compose
	// *existing* Permissions, they don't invent new ones" - every
	// requested permission must already be in the fixed enum.
	permissions := make([]domain.Permission, 0, len(in.Permissions))
	for _, p := range in.Permissions {
		permission := domain.Permission(p)
		if !domain.AllPermissions[permission] {
			return nil, &domain.ValidationError{Message: "unknown permission: " + p}
		}
		permissions = append(permissions, permission)
	}

	role, err := domain.NewRole(in.OrganizationID, in.Name, permissions)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, role); err != nil {
		return nil, err
	}

	return role, nil
}
