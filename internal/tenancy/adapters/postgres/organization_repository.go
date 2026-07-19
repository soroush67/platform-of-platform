// Package postgres is the wire-protocol adapter name per
// docs/architecture/18-backend-structure.md §2 - the real engine behind
// it is CockroachDB (docs/architecture/05-database.md §0), which speaks
// the same wire protocol, so the adapter name describes the protocol it
// implements against, not a claim about which binary is running.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/platform/outbox"
	"platform-of-platform/internal/tenancy/domain"
)

type OrganizationRepository struct {
	pool *pgxpool.Pool
}

func NewOrganizationRepository(pool *pgxpool.Pool) *OrganizationRepository {
	return &OrganizationRepository{pool: pool}
}

// Create inserts a new Organization row. Scopes app.current_org_id to the
// row being created, for the duration of this transaction only
// (set_config's third argument, is_local=true - verified against a real
// CockroachDB node to actually reset at COMMIT, not leak to the next
// request that reuses this pooled connection, per
// docs/architecture/05-database.md §0/open question #1) - the row being
// created is, by construction, the only row this session is ever allowed
// to see, so satisfying the organizations_isolation RLS policy's WITH
// CHECK for this INSERT doesn't require any broader privilege than
// creating exactly this one org.
func (r *OrganizationRepository) Create(ctx context.Context, org *domain.Organization, createdByUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, org.ID); err != nil {
		return err
	}

	settings, err := json.Marshal(org.Settings)
	if err != nil {
		return err
	}
	quota, err := json.Marshal(org.Quota)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO organizations (id, name, slug, settings, quota, status, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		org.ID, org.Name, org.Slug, settings, quota, org.Status, org.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrOrganizationSlugTaken
		}
		return err
	}

	// Same transaction as the INSERT above - the Transactional Outbox
	// pattern's whole point (internal/platform/outbox's own doc
	// comment): this event and the org row commit or roll back together.
	err = outbox.Write(ctx, tx, org.ID, "OrganizationCreated", map[string]any{
		"actor":       createdByUserID,
		"target_type": "organization",
		"target_id":   org.ID,
		"name":        org.Name,
		"slug":        org.Slug,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetByID reads back one Organization. Uses the same set_config(...,
// true)-inside-a-transaction scoping as Create - is_local=true only
// actually scopes to "this transaction" if there *is* one; called outside
// an explicit BEGIN/COMMIT, the setting would revert before the SELECT
// ever ran (each unwrapped statement is its own implicit transaction).
// The WHERE id = $1 alongside the RLS policy is deliberate belt-and-
// braces, not redundant: it's what turns "RLS hid every row" and
// "genuinely zero rows" into the same observable pgx.ErrNoRows either
// way, rather than one path returning some *other* visible org's row by
// accident if this method were ever called without setting the session
// variable first.
func (r *OrganizationRepository) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, id); err != nil {
		return nil, err
	}

	var org domain.Organization
	var settings, quota []byte
	err = tx.QueryRow(ctx,
		`SELECT id, name, slug, settings, quota, status, archived_at, created_at FROM organizations WHERE id = $1`,
		id,
	).Scan(&org.ID, &org.Name, &org.Slug, &settings, &quota, &org.Status, &org.ArchivedAt, &org.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrOrganizationNotFound
		}
		return nil, err
	}

	if err := json.Unmarshal(settings, &org.Settings); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(quota, &org.Quota); err != nil {
		return nil, err
	}

	return &org, tx.Commit(ctx)
}

// Archive implements docs/architecture/13-module-identity-rbac-tenancy.md
// §1's "DELETE /orgs/{org} sets status: archived" - a real UPDATE, not a
// row delete, so every foreign key into this org (RLS, Audit, every
// other context's organization_id) stays resolvable, matching the exact
// reasoning that doc section gives for why this can't be a hard DELETE.
func (r *OrganizationRepository) Archive(ctx context.Context, org *domain.Organization, archivedByUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, org.ID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE organizations SET status = $2, archived_at = $3 WHERE id = $1`,
		org.ID, org.Status, org.ArchivedAt,
	)
	if err != nil {
		return err
	}

	err = outbox.Write(ctx, tx, org.ID, "OrganizationArchived", map[string]any{
		"actor":       archivedByUserID,
		"target_type": "organization",
		"target_id":   org.ID,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// IsArchived is the narrow, cheap check the write paths this codebase
// actually gates on (CreateWorkspaceService, CreateVariableService,
// TriggerRunService) use - a single status column read, not a full
// GetByID + json.Unmarshal of settings/quota those callers don't need.
func (r *OrganizationRepository) IsArchived(ctx context.Context, organizationID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM organizations WHERE id = $1`, organizationID).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, domain.ErrOrganizationNotFound
		}
		return false, err
	}

	return status == domain.OrganizationStatusArchived, tx.Commit(ctx)
}

// FindOrganizationsPastPurgeWindow is the Purge Reaper's own query -
// docs/architecture/13-module-identity-rbac-tenancy.md §1: "schedules a
// background purge job 30 days out." Deliberately reads outbox_events,
// NOT the organizations table directly - organizations has FORCE ROW
// LEVEL SECURITY (migrations/0001_init.up.sql), so a plain query against
// it from the platform_app connection with no app.current_org_id set
// would silently return zero rows for every org, not an error (found
// for real: the first version of this method queried `organizations`
// directly and the reaper never purged anything, no error logged
// either, until this was traced back to RLS). outbox_events has
// deliberately no RLS (migrations/0007_outbox_audit.up.sql's own
// comment on why) - the exact same reason execution's own
// FindStaleApplyingRuns reads outbox_events instead of the (also
// RLS-protected) runs table for its cross-org scan. Archive() already
// writes an OrganizationArchived event with the org id as target_id;
// this is a real, working cross-org read of that event, not the
// underlying table.
func (r *OrganizationRepository) FindOrganizationsPastPurgeWindow(ctx context.Context, archivedBefore time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT payload->>'target_id' FROM outbox_events WHERE event_type = 'OrganizationArchived' AND occurred_at < $1`,
		archivedBefore,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// Purge is the real, hard-delete second half of the two-stage removal
// docs/architecture/13-module-identity-rbac-tenancy.md §1 describes -
// Archive (above) is the reversible soft-delete an Owner triggers
// directly; Purge is what PurgeReaperService calls once an org has sat
// archived past the grace window (and, since DeleteOrganizationService,
// what a direct operator-triggered hard-delete calls immediately,
// skipping the grace window entirely - Purge itself doesn't care which
// caller invoked it). It's genuinely irreversible (no outbox event - by
// the time this runs, deleting outbox_events is one of this very
// method's own steps, so there's nothing left to write an event
// *into*). Still needs the same set_config(...)-scoped-transaction every
// other org-scoped write in this codebase uses: every table here except
// outbox_events has FORCE ROW LEVEL SECURITY
// (migrations/0001_init.up.sql onward) - without app.current_org_id set,
// platform_app can't see or delete a single row in any of them, RLS
// silently narrows every DELETE to zero rows instead of erroring (found
// for real while verifying this - the first version omitted this and
// every delete quietly matched nothing). Order matters: every table is
// deleted in an order that respects its own foreign keys into ones
// deleted after it (migrations/0001-0012's own dependency graph, read
// directly rather than guessed) - runs before workspaces, role_bindings
// before roles, operations before compose_files/machines, etc. `users`
// is deliberately NOT touched: User is platform-global
// (docs/architecture/03-domain-model.md §3), a purged org's members
// simply lose their OrganizationMembership row, they don't cease to
// exist as Users.
//
// Secrets (0018)/Service Account+API Key (0017)/Fleet (0019) tables were
// added to this list later than the original 0001-0012 set above - found
// missing (not just guessed) while wiring the new direct hard-delete
// path: without these, Purge would hit a real FK violation on the final
// `DELETE FROM organizations` for any org with a secret mount, machine,
// compose file, or service account. compose_file_networks/
// compose_file_volumes/fleet_variables all have `ON DELETE CASCADE` into
// compose_files already (migrations/0019_fleet.up.sql) - listed
// explicitly anyway, matching this method's existing "every step named,
// nothing left implicit" style rather than relying on cascade silently
// doing it. migrations/0021 grants the DELETE privilege on secret_mounts/
// compose_files/operations this newly needs (0017/0019 already granted
// the rest).
func (r *OrganizationRepository) Purge(ctx context.Context, organizationID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	statements := []string{
		`DELETE FROM runs WHERE organization_id = $1`,
		`DELETE FROM outbox_events WHERE organization_id = $1`,
		`DELETE FROM idempotency_keys WHERE organization_id = $1`,
		`DELETE FROM variables WHERE organization_id = $1`,
		`DELETE FROM secret_mounts WHERE organization_id = $1`,
		`DELETE FROM operations WHERE organization_id = $1`,
		`DELETE FROM fleet_variables WHERE organization_id = $1`,
		`DELETE FROM compose_file_volumes WHERE organization_id = $1`,
		`DELETE FROM compose_file_networks WHERE organization_id = $1`,
		`DELETE FROM compose_files WHERE organization_id = $1`,
		`DELETE FROM machines WHERE organization_id = $1`,
		`DELETE FROM networks WHERE organization_id = $1`,
		`DELETE FROM volumes WHERE organization_id = $1`,
		`DELETE FROM api_keys WHERE organization_id = $1`,
		`DELETE FROM service_accounts WHERE organization_id = $1`,
		`DELETE FROM workspaces WHERE organization_id = $1`,
		`DELETE FROM environments WHERE organization_id = $1`,
		`DELETE FROM team_memberships WHERE organization_id = $1`,
		`DELETE FROM teams WHERE organization_id = $1`,
		`DELETE FROM role_bindings WHERE organization_id = $1`,
		`DELETE FROM roles WHERE organization_id = $1`,
		`DELETE FROM projects WHERE organization_id = $1`,
		`DELETE FROM organization_memberships WHERE organization_id = $1`,
		`DELETE FROM audit_entries WHERE organization_id = $1`,
		`DELETE FROM organizations WHERE id = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, organizationID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
