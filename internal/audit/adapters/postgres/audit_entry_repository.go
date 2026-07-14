package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/audit/domain"
)

type AuditEntryRepository struct {
	pool *pgxpool.Pool
}

func NewAuditEntryRepository(pool *pgxpool.Pool) *AuditEntryRepository {
	return &AuditEntryRepository{pool: pool}
}

// Create is the only write method this type has - no Update, no
// Delete, matching the platform_app role's own grant on audit_entries
// (SELECT, INSERT only - migrations/0007_outbox_audit.up.sql). Called
// only from the Relay's dispatch loop (via RecordEntryService), never
// from an HTTP request - there is deliberately no
// "POST /audit-log" route anywhere in this codebase.
func (r *AuditEntryRepository) Create(ctx context.Context, entry *domain.Entry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, entry.OrganizationID); err != nil {
		return err
	}

	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}

	// ON CONFLICT (source_event_id) DO NOTHING is what makes a redelivered
	// event (the Relay's own at-least-once guarantee) a safe no-op instead
	// of a duplicate row - migrations/0008_audit_idempotency.up.sql's
	// whole reason to exist.
	_, err = tx.Exec(ctx,
		`INSERT INTO audit_entries (id, organization_id, source_event_id, actor, action, target_type, target_id, metadata, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (source_event_id) DO NOTHING`,
		entry.ID, entry.OrganizationID, entry.SourceEventID, entry.Actor, entry.Action, entry.TargetType, entry.TargetID, metadata, entry.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListByOrganization is a keyset (cursor) query, not OFFSET-based - see
// ListAuditEntriesService's own comment on why. beforeCreatedAt/beforeID
// nil means "first page." The `(created_at, id) < ($2, $3)` row-value
// comparison is what makes the cursor stable even when multiple entries
// share the same created_at (a real possibility - several audit events
// from one request can commit in the same instant): ordering by
// (created_at DESC, id DESC) and comparing the pair, not created_at
// alone, means no entry is ever skipped or repeated across a page
// boundary.
func (r *AuditEntryRepository) ListByOrganization(ctx context.Context, organizationID string, limit int, beforeCreatedAt *time.Time, beforeID *string) ([]*domain.Entry, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, actor, action, target_type, target_id, metadata, created_at
		 FROM audit_entries
		 WHERE organization_id = $1
		   AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3::uuid))
		 ORDER BY created_at DESC, id DESC
		 LIMIT $4`,
		organizationID, beforeCreatedAt, beforeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*domain.Entry
	for rows.Next() {
		var e domain.Entry
		var metadata []byte
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.Actor, &e.Action, &e.TargetType, &e.TargetID, &metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &e.Metadata); err != nil {
			return nil, err
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, tx.Commit(ctx)
}
