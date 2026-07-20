package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

// Each Fleet nav menu (Machines / Networks & volumes / Compose files /
// Operations) is gated by its own independent permission pair (plus
// Operations' deploy tier) - replaces the earlier shared
// fleet:read/manage/deploy, which made it impossible to grant "manage
// Machines" without also granting "manage Operations."
// The *:delete permissions below are each stricter than their own
// *:manage sibling (Owner-only in BuiltinRoles) - same narrowing
// organization:delete already established, extended to every Fleet
// resource that has a real delete action (see internal/rbac/domain/
// role.go's own BuiltinRoles comment).
const (
	permissionMachineRead         = "machine:read"
	permissionMachineManage       = "machine:manage"
	permissionMachineDelete       = "machine:delete"
	permissionNetworkVolumeRead   = "network_volume:read"
	permissionNetworkVolumeManage = "network_volume:manage"
	permissionNetworkVolumeDelete = "network_volume:delete"
	permissionComposeFileRead     = "compose_file:read"
	permissionComposeFileManage   = "compose_file:manage"
	permissionComposeFileDelete   = "compose_file:delete"
	permissionOperationRead       = "operation:read"
	permissionOperationDeploy     = "operation:deploy"
)

type CreateNetworkInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	External         bool
}

type CreateNetworkService struct {
	repo        NetworkRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewCreateNetworkService(repo NetworkRepository, membership MembershipChecker, permChecker PermissionChecker) *CreateNetworkService {
	return &CreateNetworkService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *CreateNetworkService) Execute(ctx context.Context, in CreateNetworkInput) (*domain.Network, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionNetworkVolumeManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	network, err := domain.NewNetwork(in.OrganizationID, in.Name, in.RequestingUserID, in.External)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, network); err != nil {
		return nil, err
	}
	return network, nil
}

type ListNetworksService struct {
	repo        NetworkRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewListNetworksService(repo NetworkRepository, membership MembershipChecker, permChecker PermissionChecker) *ListNetworksService {
	return &ListNetworksService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *ListNetworksService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Network, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionNetworkVolumeRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListByOrganization(ctx, organizationID)
}

type DeleteNetworkService struct {
	repo        NetworkRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDeleteNetworkService(repo NetworkRepository, membership MembershipChecker, permChecker PermissionChecker) *DeleteNetworkService {
	return &DeleteNetworkService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *DeleteNetworkService) Execute(ctx context.Context, organizationID, requestingUserID, networkID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionNetworkVolumeDelete)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	// repo.Delete returns domain.ErrNetworkInUse on a real FK violation
	// (still attached to a ComposeFile) - a 409, not a 500, matching the
	// Python original's own catch-IntegrityError-map-to-409 behavior.
	return s.repo.Delete(ctx, requestingUserID, organizationID, networkID)
}
