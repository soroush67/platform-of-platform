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
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	return nil
}

// checkRead gates the two List* methods below - attaching/detaching a
// Network or Volume to a ComposeFile is only ever reached from
// ComposeFileDetailPage, so it's gated by compose_file:* like the rest
// of that page, not network_volume:*.
func (s *AttachmentService) checkRead(ctx context.Context, organizationID, requestingUserID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileRead)
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
	if err := s.checkRead(ctx, organizationID, requestingUserID); err != nil {
		return nil, err
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
	if err := s.checkRead(ctx, organizationID, requestingUserID); err != nil {
		return nil, err
	}
	return s.repo.ListVolumesForComposeFile(ctx, organizationID, composeFileID)
}
