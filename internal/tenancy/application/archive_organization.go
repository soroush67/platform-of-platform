package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

const permissionOrganizationDelete = "organization:delete"

// ArchiveOrganizationRepository is the narrow extra port
// ArchiveOrganizationService needs beyond OrganizationRepository's
// existing Create/GetByID - kept separate rather than widening
// OrganizationRepository itself, since no other existing caller of that
// interface needs Archive.
type ArchiveOrganizationRepository interface {
	Archive(ctx context.Context, org *domain.Organization, archivedByUserID string) error
}

// ArchiveOrganizationInput implements `DELETE /orgs/{org}`
// (docs/architecture/13-module-identity-rbac-tenancy.md §1: "sets
// status: archived... schedules a background purge job 30 days out").
// Only the soft-delete half is built here - the purge reaper is a real,
// separate, not-yet-built piece (see internal/rbac/domain/role.go's own
// comment on why organization:delete is Owner-only: this is the first
// real capability that distinguishes Owner from Admin).
type ArchiveOrganizationInput struct {
	OrganizationID   string
	RequestingUserID string
}

type ArchiveOrganizationService struct {
	orgRepo     OrganizationRepository
	archiveRepo ArchiveOrganizationRepository
	membership  MembershipRepository
	permChecker PermissionChecker
}

func NewArchiveOrganizationService(orgRepo OrganizationRepository, archiveRepo ArchiveOrganizationRepository, membership MembershipRepository, permChecker PermissionChecker) *ArchiveOrganizationService {
	return &ArchiveOrganizationService{orgRepo: orgRepo, archiveRepo: archiveRepo, membership: membership, permChecker: permChecker}
}

func (s *ArchiveOrganizationService) Execute(ctx context.Context, in ArchiveOrganizationInput) (*domain.Organization, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationDelete)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	org, err := s.orgRepo.GetByID(ctx, in.OrganizationID)
	if err != nil {
		return nil, err
	}

	if err := org.Archive(); err != nil {
		return nil, err
	}

	if err := s.archiveRepo.Archive(ctx, org, in.RequestingUserID); err != nil {
		return nil, err
	}

	return org, nil
}
