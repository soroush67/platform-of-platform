// Package postgres - see tenancy's identically-named package for why
// "postgres" names the wire protocol, not the actual engine
// (CockroachDB, docs/architecture/05-database.md §0).
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/outbox"
)

type NetworkRepository struct {
	pool *pgxpool.Pool
}

func NewNetworkRepository(pool *pgxpool.Pool) *NetworkRepository {
	return &NetworkRepository{pool: pool}
}

func (r *NetworkRepository) Create(ctx context.Context, n *domain.Network) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, n.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO networks (id, organization_id, name, external, created_by, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		n.ID, n.OrganizationID, n.Name, n.External, n.CreatedBy, n.CreatedAt,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, n.OrganizationID, "FleetNetworkCreated", map[string]any{
		"actor": n.CreatedBy, "target_type": "network", "target_id": n.ID, "name": n.Name,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *NetworkRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Network, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	n, err := scanNetwork(tx.QueryRow(ctx,
		`SELECT id, organization_id, name, external, created_by, created_at FROM networks WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNetworkNotFound
		}
		return nil, err
	}
	return n, tx.Commit(ctx)
}

func (r *NetworkRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Network, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, name, external, created_by, created_at FROM networks WHERE organization_id = $1 ORDER BY created_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var networks []*domain.Network
	for rows.Next() {
		n, err := scanNetwork(rows)
		if err != nil {
			return nil, err
		}
		networks = append(networks, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return networks, tx.Commit(ctx)
}

// Delete returns domain.ErrNetworkInUse on a real 23503 foreign-key
// violation (still attached to a ComposeFile via compose_file_networks)
// - a 409 the HTTP handler maps, not a 500, matching the ported Python
// product's own catch-IntegrityError behavior on the same operation.
func (r *NetworkRepository) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM networks WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.ErrNetworkInUse
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNetworkNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetNetworkDeleted", map[string]any{
		"actor": actorUserID, "target_type": "network", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanNetwork(row rowScanner) (*domain.Network, error) {
	var n domain.Network
	err := row.Scan(&n.ID, &n.OrganizationID, &n.Name, &n.External, &n.CreatedBy, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}
