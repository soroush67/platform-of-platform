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

type VolumeRepository struct {
	pool *pgxpool.Pool
}

func NewVolumeRepository(pool *pgxpool.Pool) *VolumeRepository {
	return &VolumeRepository{pool: pool}
}

func (r *VolumeRepository) Create(ctx context.Context, v *domain.Volume) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, v.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO volumes (id, organization_id, name, host_path, created_by, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		v.ID, v.OrganizationID, v.Name, v.HostPath, v.CreatedBy, v.CreatedAt,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, v.OrganizationID, "FleetVolumeCreated", map[string]any{
		"actor": v.CreatedBy, "target_type": "volume", "target_id": v.ID, "name": v.Name,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *VolumeRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Volume, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	v, err := scanVolume(tx.QueryRow(ctx,
		`SELECT id, organization_id, name, host_path, created_by, created_at FROM volumes WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrVolumeNotFound
		}
		return nil, err
	}
	return v, tx.Commit(ctx)
}

func (r *VolumeRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Volume, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, name, host_path, created_by, created_at FROM volumes WHERE organization_id = $1 ORDER BY created_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var volumes []*domain.Volume
	for rows.Next() {
		v, err := scanVolume(rows)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return volumes, tx.Commit(ctx)
}

func (r *VolumeRepository) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM volumes WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.ErrVolumeInUse
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrVolumeNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetVolumeDeleted", map[string]any{
		"actor": actorUserID, "target_type": "volume", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func scanVolume(row rowScanner) (*domain.Volume, error) {
	var v domain.Volume
	err := row.Scan(&v.ID, &v.OrganizationID, &v.Name, &v.HostPath, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
