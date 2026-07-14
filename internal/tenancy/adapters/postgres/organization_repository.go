// Package postgres is the wire-protocol adapter name per
// docs/architecture/18-backend-structure.md §2 - the real engine behind
// it is CockroachDB (docs/architecture/05-database.md §0), which speaks
// the same wire protocol, so the adapter name describes the protocol it
// implements against, not a claim about which binary is running.
package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

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
func (r *OrganizationRepository) Create(ctx context.Context, org *domain.Organization) error {
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
		`INSERT INTO organizations (id, name, slug, settings, quota, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		org.ID, org.Name, org.Slug, settings, quota, org.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
