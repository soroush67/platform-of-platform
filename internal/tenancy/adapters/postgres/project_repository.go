package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrProjectAlreadyExists
		}
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

// ProjectExists is Tenancy's side of the cross-context ProjectChecker
// port the Workspace context declares itself
// (internal/workspace/application/ports.go) - returns a bool, not
// *domain.Project, deliberately: Workspace must never import
// tenancy/domain (docs/architecture/18-backend-structure.md §3), and a
// bool is all a "does this project genuinely belong to this org" check
// needs to hand back across the context boundary.
func (r *ProjectRepository) ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM projects WHERE organization_id = $1 AND id = $2)`,
		organizationID, projectID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}

// Purge is a genuine hard delete of a Project and everything scoped
// under it - unlike OrganizationRepository.Purge, most of these tables
// aren't keyed directly by project_id, so this deletes through
// subqueries against this project's own workspaces/environments first.
// Deliberately does NOT touch compose_files themselves (org-scoped,
// possibly linked to other projects too) - only the compose_file_projects
// link rows for this project.
func (r *ProjectRepository) Purge(ctx context.Context, organizationID, projectID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	// Every statement below binds projectID as its only placeholder ($1) -
	// CockroachDB (unlike some Postgres drivers) fails a query with a
	// placeholder that's never referenced in its own text ("could not
	// determine data type of placeholder $1", SQLSTATE 42P18), so
	// organizationID can't just ride along as an always-passed $1/$2 pair
	// the way OrganizationRepository.Purge's single-param statements do.
	// Safe to scope by projectID alone here - app.current_org_id (set
	// above) already makes every one of these tables' own RLS policy do
	// the organization-scoping; the final statement re-adds organization_id
	// explicitly anyway, for the same belt-and-braces reasoning
	// GetByID's own doc comment gives.
	statements := []string{
		`DELETE FROM runs WHERE workspace_id IN (SELECT id FROM workspaces WHERE project_id = $1)`,
		`DELETE FROM variables WHERE
			(scope_type = 'project' AND scope_id = $1) OR
			(scope_type = 'environment' AND scope_id IN (SELECT id FROM environments WHERE project_id = $1)) OR
			(scope_type = 'workspace' AND scope_id IN (SELECT id FROM workspaces WHERE project_id = $1))`,
		`DELETE FROM role_bindings WHERE
			(scope_type = 'project' AND scope_id = $1) OR
			(scope_type = 'workspace' AND scope_id IN (SELECT id FROM workspaces WHERE project_id = $1))`,
		`DELETE FROM compose_file_projects WHERE project_id = $1`,
		`DELETE FROM workspaces WHERE project_id = $1`,
		`DELETE FROM environments WHERE project_id = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, projectID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE organization_id = $1 AND id = $2`, organizationID, projectID); err != nil {
		return err
	}

	return tx.Commit(ctx)
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
