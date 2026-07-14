package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/workspace/domain"
)

type WorkspaceRepository struct {
	pool *pgxpool.Pool
}

func NewWorkspaceRepository(pool *pgxpool.Pool) *WorkspaceRepository {
	return &WorkspaceRepository{pool: pool}
}

func (r *WorkspaceRepository) Create(ctx context.Context, ws *domain.Workspace) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, ws.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO workspaces (id, organization_id, project_id, environment_id, name, execution_engine,
		  vcs_link_id, current_state_version_id, locked, locked_by_run_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		ws.ID, ws.OrganizationID, ws.ProjectID, ws.EnvironmentID, ws.Name, string(ws.ExecutionEngine),
		ws.VCSLinkID, ws.CurrentStateVersionID, ws.Locked, ws.LockedByRunID, ws.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *WorkspaceRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Workspace, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var w domain.Workspace
	err = tx.QueryRow(ctx,
		`SELECT id, organization_id, project_id, environment_id, name, execution_engine,
		  vcs_link_id, current_state_version_id, locked, locked_by_run_id, created_at
		 FROM workspaces WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	).Scan(&w.ID, &w.OrganizationID, &w.ProjectID, &w.EnvironmentID, &w.Name, &w.ExecutionEngine,
		&w.VCSLinkID, &w.CurrentStateVersionID, &w.Locked, &w.LockedByRunID, &w.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrWorkspaceNotFound
		}
		return nil, err
	}

	return &w, tx.Commit(ctx)
}

func (r *WorkspaceRepository) ListByProject(ctx context.Context, organizationID, projectID string) ([]*domain.Workspace, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, project_id, environment_id, name, execution_engine,
		  vcs_link_id, current_state_version_id, locked, locked_by_run_id, created_at
		 FROM workspaces WHERE organization_id = $1 AND project_id = $2 ORDER BY created_at`,
		organizationID, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []*domain.Workspace
	for rows.Next() {
		var w domain.Workspace
		if err := rows.Scan(&w.ID, &w.OrganizationID, &w.ProjectID, &w.EnvironmentID, &w.Name, &w.ExecutionEngine,
			&w.VCSLinkID, &w.CurrentStateVersionID, &w.Locked, &w.LockedByRunID, &w.CreatedAt); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return workspaces, tx.Commit(ctx)
}
