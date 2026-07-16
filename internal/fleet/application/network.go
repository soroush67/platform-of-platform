package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

const (
	permissionFleetRead   = "fleet:read"
	permissionFleetManage = "fleet:manage"
	permissionFleetDeploy = "fleet:deploy"
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
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionFleetManage)
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
	repo       NetworkRepository
	membership MembershipChecker
}

func NewListNetworksService(repo NetworkRepository, membership MembershipChecker) *ListNetworksService {
	return &ListNetworksService{repo: repo, membership: membership}
}

func (s *ListNetworksService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Network, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
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
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionFleetManage)
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
