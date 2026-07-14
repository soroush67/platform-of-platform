// Package postgres is the wire-protocol adapter name per
// docs/architecture/18-backend-structure.md §2 - the real engine behind
// it is CockroachDB (docs/architecture/05-database.md §0), which speaks
// the same wire protocol, so the adapter name describes the protocol it
// implements against, not a claim about which binary is running.
package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
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
