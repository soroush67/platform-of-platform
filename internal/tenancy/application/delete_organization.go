package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// DeleteOrganizationRepository is the narrow extra port
// DeleteOrganizationService needs beyond OrganizationRepository's own
// Create/GetByID - Purge is the same hard-delete OrganizationRepository
// (postgres adapter) already implements for PurgeReaperService, reused
// here rather than duplicated.
type DeleteOrganizationRepository interface {
	Purge(ctx context.Context, organizationID string) error
}

// DeleteOrganizationInput is a genuinely irreversible hard delete - not
// the two-stage Archive-then-wait-30-days flow ArchiveOrganizationService
// implements. Operator-confirmed scope: works on an Organization
// regardless of its current status (active or already archived) - no
// "must archive first" gate, since an operator who already knows they
// want everything gone shouldn't be forced through the reversible path
// first. Uses the same organization:delete permission (Owner-only) as
// Archive - no new, stricter permission was asked for.
type DeleteOrganizationInput struct {
	OrganizationID   string
	RequestingUserID string
}

type DeleteOrganizationService struct {
	orgRepo     OrganizationRepository
	deleteRepo  DeleteOrganizationRepository
	membership  MembershipRepository
	permChecker PermissionChecker
}

func NewDeleteOrganizationService(orgRepo OrganizationRepository, deleteRepo DeleteOrganizationRepository, membership MembershipRepository, permChecker PermissionChecker) *DeleteOrganizationService {
	return &DeleteOrganizationService{orgRepo: orgRepo, deleteRepo: deleteRepo, membership: membership, permChecker: permChecker}
}

func (s *DeleteOrganizationService) Execute(ctx context.Context, in DeleteOrganizationInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationDelete)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	// Confirms the org genuinely exists (and this session can resolve
	// it) before calling Purge - the same reasoning Archive's own
	// Execute calls GetByID for, rather than letting Purge's own
	// RLS-scoped deletes just quietly match zero rows for an id that was
	// never real to begin with.
	if _, err := s.orgRepo.GetByID(ctx, in.OrganizationID); err != nil {
		return err
	}

	return s.deleteRepo.Purge(ctx, in.OrganizationID)
}
