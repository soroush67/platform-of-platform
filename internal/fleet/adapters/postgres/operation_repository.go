package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/outbox"
)

// OperationRepository is app-pool-backed (RLS-scoped) - every method
// here operates within one Organization, same as every other Fleet
// repository. Cross-org discovery of queued candidates lives in
// OperationScanner (operation_scanner.go) instead, root-pool-backed for
// exactly the same reason StaleRunReaperService's own two-tier design
// already established.
type OperationRepository struct {
	pool *pgxpool.Pool
}

func NewOperationRepository(pool *pgxpool.Pool) *OperationRepository {
	return &OperationRepository{pool: pool}
}

func (r *OperationRepository) Create(ctx context.Context, o *domain.Operation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, o.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO operations (id, organization_id, compose_file_id, machine_id, operation_type, status, triggered_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		o.ID, o.OrganizationID, o.ComposeFileID, o.MachineID, string(o.OperationType), string(o.Status), o.TriggeredBy, o.CreatedAt,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, o.OrganizationID, "FleetOperationCreated", map[string]any{
		"actor": o.TriggeredBy, "target_type": "operation", "target_id": o.ID,
		"operation_type": string(o.OperationType), "machine_id": o.MachineID, "compose_file_id": o.ComposeFileID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *OperationRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Operation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	o, err := scanOperation(tx.QueryRow(ctx, operationSelectColumns+` FROM operations WHERE organization_id = $1 AND id = $2`, organizationID, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrOperationNotFound
		}
		return nil, err
	}
	return o, tx.Commit(ctx)
}

func (r *OperationRepository) ListByOrganization(ctx context.Context, organizationID, composeFileID, machineID string) ([]*domain.Operation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	query := operationSelectColumns + ` FROM operations WHERE organization_id = $1`
	args := []any{organizationID}
	if composeFileID != "" {
		args = append(args, composeFileID)
		query += ` AND compose_file_id = $` + strconv.Itoa(len(args))
	}
	if machineID != "" {
		args = append(args, machineID)
		query += ` AND machine_id = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operations []*domain.Operation
	for rows.Next() {
		o, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return operations, tx.Commit(ctx)
}

// TryClaim is the atomic compare-and-swap DeployExecutor uses to take
// ownership of a queued Operation - identical shape to Execution's own
// TryStartApplying, needed for the exact same reason (two poll ticks,
// possibly across replicas eventually, must not both execute the same
// row).
func (r *OperationRepository) TryClaim(ctx context.Context, organizationID, id string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE operations SET status = 'running', started_at = now() WHERE organization_id = $1 AND id = $2 AND status = 'queued'`,
		organizationID, id,
	)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, tx.Commit(ctx)
	}

	return true, tx.Commit(ctx)
}

func (r *OperationRepository) MarkFinished(ctx context.Context, organizationID, id string, status domain.OperationStatus, exitCode *int, output string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE operations SET status = $3, finished_at = now(), exit_code = $4, output = $5 WHERE organization_id = $1 AND id = $2`,
		organizationID, id, string(status), exitCode, output,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOperationNotFound
	}

	return tx.Commit(ctx)
}

const operationSelectColumns = `SELECT id, organization_id, compose_file_id, machine_id, operation_type, status, triggered_by, created_at, started_at, finished_at, exit_code, output`

func scanOperation(row rowScanner) (*domain.Operation, error) {
	var o domain.Operation
	var opType, status string
	err := row.Scan(&o.ID, &o.OrganizationID, &o.ComposeFileID, &o.MachineID, &opType, &status, &o.TriggeredBy,
		&o.CreatedAt, &o.StartedAt, &o.FinishedAt, &o.ExitCode, &o.Output)
	if err != nil {
		return nil, err
	}
	o.OperationType = domain.OperationType(opType)
	o.Status = domain.OperationStatus(status)
	return &o, nil
}
