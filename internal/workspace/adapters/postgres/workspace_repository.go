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

// GetExecutionEngine is the Workspace side of Execution's own
// WorkspaceEngineReader port - returns just the engine string, not the
// full domain.Workspace, same "never leak a domain type across the
// context boundary" reasoning as GetScope/Exists above.
func (r *WorkspaceRepository) GetExecutionEngine(ctx context.Context, organizationID, workspaceID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return "", err
	}

	var engine string
	err = tx.QueryRow(ctx,
		`SELECT execution_engine FROM workspaces WHERE organization_id = $1 AND id = $2`,
		organizationID, workspaceID,
	).Scan(&engine)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", domain.ErrWorkspaceNotFound
		}
		return "", err
	}

	return engine, tx.Commit(ctx)
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

// WorkspaceExists is the Workspace side of the cross-context
// WorkspaceChecker port the Execution context declares itself - same
// "return a bool across the context boundary, never the domain type"
// reasoning as Tenancy's ProjectExists.
func (r *WorkspaceRepository) WorkspaceExists(ctx context.Context, organizationID, projectID, workspaceID string) (bool, error) {
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
		`SELECT EXISTS(SELECT 1 FROM workspaces WHERE organization_id = $1 AND project_id = $2 AND id = $3)`,
		organizationID, projectID, workspaceID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}

// Exists and GetScope are lighter-weight cross-context checks than
// WorkspaceExists/GetByID above - the Variables context (which declares
// its own ScopeChecker/WorkspaceScopeReader ports) only ever has a
// workspace id on hand, not also its parent project id the way
// Execution's URL structure guarantees, so it needs a check/read that
// doesn't require one.
func (r *WorkspaceRepository) Exists(ctx context.Context, organizationID, workspaceID string) (bool, error) {
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
		`SELECT EXISTS(SELECT 1 FROM workspaces WHERE organization_id = $1 AND id = $2)`,
		organizationID, workspaceID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}

// GetScope returns just the two ancestor ids the Variables cascade
// needs to walk (docs/architecture/03-domain-model.md §7) - projectID
// and, if any, environmentID - never the full domain.Workspace, per the
// "never leak a domain type across the context boundary" rule already
// applied to every other cross-context port in this codebase.
func (r *WorkspaceRepository) GetScope(ctx context.Context, organizationID, workspaceID string) (projectID string, environmentID *string, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return "", nil, err
	}

	err = tx.QueryRow(ctx,
		`SELECT project_id, environment_id FROM workspaces WHERE organization_id = $1 AND id = $2`,
		organizationID, workspaceID,
	).Scan(&projectID, &environmentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil, domain.ErrWorkspaceNotFound
		}
		return "", nil, err
	}

	return projectID, environmentID, tx.Commit(ctx)
}

// TryLock is the real implementation of docs/architecture/05-database.md
// §2's "the workspace lock's enforcement... is a Postgres SELECT ... FOR
// UPDATE inside the transaction that transitions a Run into a running
// status, not a separate lock table" - and the Execution context's own
// WorkspaceLocker port (internal/execution/application/ports.go).
// Returns (false, nil) - not an error - if the workspace was already
// locked by a different run, same (bool, error) shape as every other
// cross-context check in this codebase (IsMember, HasPermission,
// ProjectExists): "the answer is no" isn't a failure.
func (r *WorkspaceRepository) TryLock(ctx context.Context, organizationID, workspaceID, runID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var locked bool
	err = tx.QueryRow(ctx,
		`SELECT locked FROM workspaces WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
		organizationID, workspaceID,
	).Scan(&locked)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, domain.ErrWorkspaceNotFound
		}
		return false, err
	}
	if locked {
		return false, nil
	}

	_, err = tx.Exec(ctx,
		`UPDATE workspaces SET locked = true, locked_by_run_id = $1 WHERE organization_id = $2 AND id = $3`,
		runID, organizationID, workspaceID,
	)
	if err != nil {
		return false, err
	}

	return true, tx.Commit(ctx)
}

// Unlock only releases the lock if runID is the run that actually holds
// it (the WHERE clause's locked_by_run_id = $3) - defense against a
// stray/late Unlock call from a run that never successfully acquired
// the lock in the first place clobbering a different run's active one.
func (r *WorkspaceRepository) Unlock(ctx context.Context, organizationID, workspaceID, runID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE workspaces SET locked = false, locked_by_run_id = NULL
		 WHERE organization_id = $1 AND id = $2 AND locked_by_run_id = $3`,
		organizationID, workspaceID, runID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Purge is a genuine hard delete of a Workspace and everything scoped
// under it (runs, workspace-scoped variables, workspace-scoped role
// bindings) - same shape as Tenancy's own ProjectRepository.Purge, one
// level down. Every statement binds workspaceID alone as $1 - the
// CockroachDB "unused placeholder" bug hit and fixed in Project's own
// Purge (an unreferenced $N makes the whole statement fail with
// SQLSTATE 42P18) applies here too, so organization_id only appears on
// the final statement, which is the only one that actually needs it.
func (r *WorkspaceRepository) Purge(ctx context.Context, organizationID, workspaceID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	statements := []string{
		`DELETE FROM runs WHERE workspace_id = $1`,
		`DELETE FROM variables WHERE scope_type = 'workspace' AND scope_id = $1`,
		`DELETE FROM role_bindings WHERE scope_type = 'workspace' AND scope_id = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, workspaceID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM workspaces WHERE organization_id = $1 AND id = $2`, organizationID, workspaceID); err != nil {
		return err
	}

	return tx.Commit(ctx)
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
