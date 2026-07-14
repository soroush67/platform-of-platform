package application

import (
	"context"

	"platform-of-platform/internal/audit/domain"
)

// AuditEntryRepository - deliberately no Update/Delete method exists
// here at all, matching the database grant
// (migrations/0007_outbox_audit.up.sql).
type AuditEntryRepository interface {
	Create(ctx context.Context, entry *domain.Entry) error
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Entry, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}
