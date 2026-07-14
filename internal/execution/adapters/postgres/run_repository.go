package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/execution/domain"
	"platform-of-platform/internal/platform/outbox"
)

type RunRepository struct {
	pool *pgxpool.Pool
}

func NewRunRepository(pool *pgxpool.Pool) *RunRepository {
	return &RunRepository{pool: pool}
}

func (r *RunRepository) Create(ctx context.Context, run *domain.Run) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, run.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO runs (id, organization_id, workspace_id, trigger, triggered_by, status,
		  plan_output_ref, apply_output_ref, created_at, started_at, finished_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		run.ID, run.OrganizationID, run.WorkspaceID, string(run.Trigger), run.TriggeredBy, string(run.Status),
		run.PlanOutputRef, run.ApplyOutputRef, run.CreatedAt, run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return err
	}

	// docs/architecture/03-domain-model.md §6: "RunQueued ... this is the
	// single busiest event stream in the system." Only RunQueued is
	// emitted here (not the full PlanCompleted/RunApplying/... set) since
	// this codebase has no Worker to ever produce those - see the
	// domain package's own comment on why RunStatus is modeled fully but
	// only queued/canceled are ever real here.
	err = outbox.Write(ctx, tx, run.OrganizationID, "RunQueued", map[string]any{
		"actor":        run.TriggeredBy,
		"target_type":  "run",
		"target_id":    run.ID,
		"workspace_id": run.WorkspaceID,
	})
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *RunRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Run, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	run, err := scanRun(tx.QueryRow(ctx,
		`SELECT id, organization_id, workspace_id, trigger, triggered_by, status,
		  plan_output_ref, apply_output_ref, created_at, started_at, finished_at
		 FROM runs WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrRunNotFound
		}
		return nil, err
	}

	return run, tx.Commit(ctx)
}

func (r *RunRepository) ListByWorkspace(ctx context.Context, organizationID, workspaceID string) ([]*domain.Run, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	// (workspace_id, status, created_at desc) - the index
	// migrations/0005_runs.up.sql adds for exactly this query shape
	// (docs/architecture/05-database.md table map, Execution row).
	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, workspace_id, trigger, triggered_by, status,
		  plan_output_ref, apply_output_ref, created_at, started_at, finished_at
		 FROM runs WHERE organization_id = $1 AND workspace_id = $2 ORDER BY created_at DESC`,
		organizationID, workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return runs, tx.Commit(ctx)
}

// Update takes actorUserID for the same reason
// OrganizationRepository.Create does - it's not a field on Run, it's
// who performed *this particular* mutation, needed only for the outbox
// event this method writes in the same transaction.
func (r *RunRepository) Update(ctx context.Context, run *domain.Run, actorUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, run.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE runs SET status = $1, plan_output_ref = $2, apply_output_ref = $3, started_at = $4, finished_at = $5
		 WHERE organization_id = $6 AND id = $7`,
		string(run.Status), run.PlanOutputRef, run.ApplyOutputRef, run.StartedAt, run.FinishedAt,
		run.OrganizationID, run.ID,
	)
	if err != nil {
		return err
	}

	// Real Run domain events (docs/architecture/03-domain-model.md §6) for
	// every terminal transition this codebase's own code actually
	// produces (Run.Cancel/MarkApplied/MarkFailed) - not every status
	// this method could theoretically see, since only those three ever
	// call Update today.
	var eventType string
	switch run.Status {
	case domain.RunStatusCanceled:
		eventType = "RunCanceled"
	case domain.RunStatusApplied:
		eventType = "RunApplied"
	case domain.RunStatusFailed:
		eventType = "RunFailed"
	}
	if eventType != "" {
		err = outbox.Write(ctx, tx, run.OrganizationID, eventType, map[string]any{
			"actor":        actorUserID,
			"target_type":  "run",
			"target_id":    run.ID,
			"workspace_id": run.WorkspaceID,
		})
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// TryStartApplying is a real atomic compare-and-swap - see the port's
// own doc comment (internal/execution/application/ports.go) for why
// RunDispatchService needs exactly this shape instead of a read-then-
// write round trip.
func (r *RunRepository) TryStartApplying(ctx context.Context, organizationID, runID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE runs SET status = 'applying', started_at = now() WHERE organization_id = $1 AND id = $2 AND status = 'queued'`,
		organizationID, runID,
	)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	err = outbox.Write(ctx, tx, organizationID, "RunApplying", map[string]any{
		"actor":       "system",
		"target_type": "run",
		"target_id":   runID,
	})
	if err != nil {
		return false, err
	}

	return true, tx.Commit(ctx)
}

// RevertToQueued is RunDispatchService's compensation when
// TryStartApplying claimed a Run but dispatch then found no connected
// Worker - see the port's own doc comment. No outbox event on the way
// back down; the original RunQueued event (still unpublished, since
// this whole call happens inside that event's own Handler returning an
// error) is what the Relay retries.
func (r *RunRepository) RevertToQueued(ctx context.Context, organizationID, runID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE runs SET status = 'queued', started_at = NULL WHERE organization_id = $1 AND id = $2 AND status = 'applying'`,
		organizationID, runID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// rowScanner - both pgx.Row (QueryRow) and pgx.Rows (Query, via its
// embedded Scan) satisfy this, letting GetByID and ListByWorkspace share
// one field-order-source-of-truth instead of two copies that could
// silently drift apart.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (*domain.Run, error) {
	var run domain.Run
	err := row.Scan(&run.ID, &run.OrganizationID, &run.WorkspaceID, &run.Trigger, &run.TriggeredBy, &run.Status,
		&run.PlanOutputRef, &run.ApplyOutputRef, &run.CreatedAt, &run.StartedAt, &run.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &run, nil
}
