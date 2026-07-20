package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

type CreateVolumeInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	HostPath         string
}

type CreateVolumeService struct {
	repo        VolumeRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewCreateVolumeService(repo VolumeRepository, membership MembershipChecker, permChecker PermissionChecker) *CreateVolumeService {
	return &CreateVolumeService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *CreateVolumeService) Execute(ctx context.Context, in CreateVolumeInput) (*domain.Volume, error) {
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

	volume, err := domain.NewVolume(in.OrganizationID, in.Name, in.HostPath, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, volume); err != nil {
		return nil, err
	}
	return volume, nil
}

type ListVolumesService struct {
	repo        VolumeRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewListVolumesService(repo VolumeRepository, membership MembershipChecker, permChecker PermissionChecker) *ListVolumesService {
	return &ListVolumesService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *ListVolumesService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Volume, error) {
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

type DeleteVolumeService struct {
	repo        VolumeRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDeleteVolumeService(repo VolumeRepository, membership MembershipChecker, permChecker PermissionChecker) *DeleteVolumeService {
	return &DeleteVolumeService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *DeleteVolumeService) Execute(ctx context.Context, organizationID, requestingUserID, volumeID string) error {
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
	return s.repo.Delete(ctx, requestingUserID, organizationID, volumeID)
}
