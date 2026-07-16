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

type ComposeFileRepository struct {
	pool *pgxpool.Pool
}

func NewComposeFileRepository(pool *pgxpool.Pool) *ComposeFileRepository {
	return &ComposeFileRepository{pool: pool}
}

// Create returns domain.ErrGlobalComposeFileExists on a real 23505
// unique-violation against compose_files_one_global_per_org (migrations/
// 0019_fleet.up.sql's own partial unique index) - the storage-layer
// enforcement of "at most one global ComposeFile per Organization"
// (decision #8 in the Fleet plan).
func (r *ComposeFileRepository) Create(ctx context.Context, c *domain.ComposeFile) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, c.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO compose_files (id, organization_id, name, is_global, compose_content, created_by, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		c.ID, c.OrganizationID, c.Name, c.IsGlobal, c.ComposeContent, c.CreatedBy, c.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "compose_files_one_global_per_org" {
			return domain.ErrGlobalComposeFileExists
		}
		return err
	}

	if err := outbox.Write(ctx, tx, c.OrganizationID, "FleetComposeFileCreated", map[string]any{
		"actor": c.CreatedBy, "target_type": "compose_file", "target_id": c.ID, "name": c.Name,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *ComposeFileRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.ComposeFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	c, err := scanComposeFile(tx.QueryRow(ctx, composeFileSelectColumns+` FROM compose_files WHERE organization_id = $1 AND id = $2`, organizationID, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrComposeFileNotFound
		}
		return nil, err
	}
	return c, tx.Commit(ctx)
}

// GetGlobal returns (nil, false, nil) when this Organization has no
// global ComposeFile yet - a real, displayable state, not an error (see
// domain.ComposeFile's own doc comment on decision #8).
func (r *ComposeFileRepository) GetGlobal(ctx context.Context, organizationID string) (*domain.ComposeFile, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, false, err
	}

	c, err := scanComposeFile(tx.QueryRow(ctx, composeFileSelectColumns+` FROM compose_files WHERE organization_id = $1 AND is_global = true`, organizationID))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, false, tx.Commit(ctx)
		}
		return nil, false, err
	}
	return c, true, tx.Commit(ctx)
}

func (r *ComposeFileRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.ComposeFile, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, composeFileSelectColumns+` FROM compose_files WHERE organization_id = $1 ORDER BY created_at`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*domain.ComposeFile
	for rows.Next() {
		c, err := scanComposeFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, tx.Commit(ctx)
}

func (r *ComposeFileRepository) UpdateContent(ctx context.Context, actorUserID, organizationID, id, content string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `UPDATE compose_files SET compose_content = $3 WHERE organization_id = $1 AND id = $2`, organizationID, id, content)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrComposeFileNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetComposeFileContentUpdated", map[string]any{
		"actor": actorUserID, "target_type": "compose_file", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const composeFileSelectColumns = `SELECT id, organization_id, name, is_global, compose_content, created_by, created_at`

func scanComposeFile(row rowScanner) (*domain.ComposeFile, error) {
	var c domain.ComposeFile
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.IsGlobal, &c.ComposeContent, &c.CreatedBy, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
