package application

import (
	"context"

	"platform-of-platform/internal/audit/domain"
)

const permissionOrganizationManage = "organization:manage"

// ListAuditEntriesService implements `GET /api/v1/orgs/{org}/audit-log`
// (docs/architecture/04-api-design.md §1's own named route). Gated by
// organization:manage, not mere membership - an audit trail is
// sensitive by nature (who did what, when), a stricter bar than the
// membership-only gate every other read in this codebase uses.
type ListAuditEntriesService struct {
	repo        AuditEntryRepository
	permChecker PermissionChecker
}

func NewListAuditEntriesService(repo AuditEntryRepository, permChecker PermissionChecker) *ListAuditEntriesService {
	return &ListAuditEntriesService{repo: repo, permChecker: permChecker}
}

func (s *ListAuditEntriesService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Entry, error) {
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListByOrganization(ctx, organizationID)
}
