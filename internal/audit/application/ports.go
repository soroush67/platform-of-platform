package application

import (
	"context"
	"time"

	"platform-of-platform/internal/audit/domain"
)

// AuditEntryRepository - deliberately no Update/Delete method exists
// here at all, matching the database grant
// (migrations/0007_outbox_audit.up.sql). ListByOrganization takes a
// keyset cursor (beforeCreatedAt/beforeID, both nil for the first page)
// instead of an OFFSET - see ListAuditEntriesService's own comment on
// why.
type AuditEntryRepository interface {
	Create(ctx context.Context, entry *domain.Entry) error
	ListByOrganization(ctx context.Context, organizationID string, limit int, beforeCreatedAt *time.Time, beforeID *string) ([]*domain.Entry, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}
