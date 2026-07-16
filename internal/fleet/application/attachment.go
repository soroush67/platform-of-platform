package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

// attachmentServices bundles the 4 attach/detach operations behind one
// small struct - each is a thin membership+permission-gated call into
// AttachmentRepository, not worth 4 separate service types the way
// Create/List/Get/Update/Delete are for a real aggregate (attachments
// are pure junction rows, no domain constructor/invariant of their own
// beyond what the repository's own FK/unique constraints enforce).
type AttachmentService struct {
	repo        AttachmentRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewAttachmentService(repo AttachmentRepository, membership MembershipChecker, permChecker PermissionChecker) *AttachmentService {
	return &AttachmentService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *AttachmentService) checkManage(ctx context.Context, organizationID, requestingUserID string) error {
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
	return nil
}

func (s *AttachmentService) AttachNetwork(ctx context.Context, organizationID, requestingUserID, composeFileID, networkID string) error {
	if err := s.checkManage(ctx, organizationID, requestingUserID); err != nil {
		return err
	}
	return s.repo.AttachNetwork(ctx, requestingUserID, organizationID, composeFileID, networkID)
}

func (s *AttachmentService) DetachNetwork(ctx context.Context, organizationID, requestingUserID, composeFileID, networkID string) error {
	if err := s.checkManage(ctx, organizationID, requestingUserID); err != nil {
		return err
	}
	return s.repo.DetachNetwork(ctx, requestingUserID, organizationID, composeFileID, networkID)
}

func (s *AttachmentService) ListNetworks(ctx context.Context, organizationID, requestingUserID, composeFileID string) ([]*domain.Network, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListNetworksForComposeFile(ctx, organizationID, composeFileID)
}

func (s *AttachmentService) AttachVolume(ctx context.Context, organizationID, requestingUserID, composeFileID, volumeID, containerPath string) error {
	if err := s.checkManage(ctx, organizationID, requestingUserID); err != nil {
		return err
	}
	if containerPath == "" {
		return &domain.ValidationError{Message: "container_path is required"}
	}
	return s.repo.AttachVolume(ctx, requestingUserID, organizationID, composeFileID, volumeID, containerPath)
}

func (s *AttachmentService) DetachVolume(ctx context.Context, organizationID, requestingUserID, composeFileID, volumeID string) error {
	if err := s.checkManage(ctx, organizationID, requestingUserID); err != nil {
		return err
	}
	return s.repo.DetachVolume(ctx, requestingUserID, organizationID, composeFileID, volumeID)
}

func (s *AttachmentService) ListVolumes(ctx context.Context, organizationID, requestingUserID, composeFileID string) ([]VolumeAttachmentView, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListVolumesForComposeFile(ctx, organizationID, composeFileID)
}
