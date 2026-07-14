package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/tenancy/domain"
)

type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

// Create - same set_config(..., true)-scoped-transaction pattern as
// OrganizationRepository.Create, scoped to the project's own
// organization_id (docs/architecture/05-database.md §1).
func (r *ProjectRepository) Create(ctx context.Context, project *domain.Project) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, project.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO projects (id, organization_id, name, slug, description, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		project.ID, project.OrganizationID, project.Name, project.Slug, project.Description, project.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetByID scopes to organizationID (the org already verified by the
// caller's membership check, per GetProjectService), then filters by
// project id too - same belt-and-braces reasoning as
// OrganizationRepository.GetByID: a project id belonging to a
// *different* org can never leak through here even if this method were
// ever called with the wrong organizationID by mistake, because RLS
// would already have hidden it before the WHERE clause got a chance to.
func (r *ProjectRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Project, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var p domain.Project
	err = tx.QueryRow(ctx,
		`SELECT id, organization_id, name, slug, description, created_at FROM projects WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}

	return &p, tx.Commit(ctx)
}

func (r *ProjectRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Project, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, name, slug, description, created_at FROM projects WHERE organization_id = $1 ORDER BY created_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return projects, tx.Commit(ctx)
}
