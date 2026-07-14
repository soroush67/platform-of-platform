// Package postgres - see tenancy's identically-named package for why
// "postgres" names the wire protocol, not the actual engine
// (CockroachDB, docs/architecture/05-database.md §0).
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/workspace/domain"
)

type EnvironmentRepository struct {
	pool *pgxpool.Pool
}

func NewEnvironmentRepository(pool *pgxpool.Pool) *EnvironmentRepository {
	return &EnvironmentRepository{pool: pool}
}

func (r *EnvironmentRepository) Create(ctx context.Context, env *domain.Environment) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, env.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO environments (id, organization_id, project_id, name, promotion_rank, requires_approval, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		env.ID, env.OrganizationID, env.ProjectID, env.Name, env.PromotionRank, env.RequiresApproval, env.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *EnvironmentRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Environment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var e domain.Environment
	err = tx.QueryRow(ctx,
		`SELECT id, organization_id, project_id, name, promotion_rank, requires_approval, created_at
		 FROM environments WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	).Scan(&e.ID, &e.OrganizationID, &e.ProjectID, &e.Name, &e.PromotionRank, &e.RequiresApproval, &e.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrEnvironmentNotFound
		}
		return nil, err
	}

	return &e, tx.Commit(ctx)
}

func (r *EnvironmentRepository) ListByProject(ctx context.Context, organizationID, projectID string) ([]*domain.Environment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, project_id, name, promotion_rank, requires_approval, created_at
		 FROM environments WHERE organization_id = $1 AND project_id = $2 ORDER BY promotion_rank, created_at`,
		organizationID, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []*domain.Environment
	for rows.Next() {
		var e domain.Environment
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.ProjectID, &e.Name, &e.PromotionRank, &e.RequiresApproval, &e.CreatedAt); err != nil {
			return nil, err
		}
		envs = append(envs, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return envs, tx.Commit(ctx)
}
