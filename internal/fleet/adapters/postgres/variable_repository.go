package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/outbox"
)

type VariableRepository struct {
	pool *pgxpool.Pool
}

func NewVariableRepository(pool *pgxpool.Pool) *VariableRepository {
	return &VariableRepository{pool: pool}
}

func (r *VariableRepository) Create(ctx context.Context, actorUserID string, v *domain.Variable) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, v.OrganizationID); err != nil {
		return err
	}

	var value, mountID, path *string
	if v.SecretRef != nil {
		mountID, path = &v.SecretRef.MountID, &v.SecretRef.Path
	} else {
		value = &v.Value
	}
	var fileTargetPath *string
	if v.FileTargetPath != "" {
		fileTargetPath = &v.FileTargetPath
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO fleet_variables (id, organization_id, compose_file_id, key, var_type, value, secret_mount_id, secret_path, file_target_path, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		v.ID, v.OrganizationID, v.ComposeFileID, v.Key, string(v.VarType), value, mountID, path, fileTargetPath, v.CreatedAt,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, v.OrganizationID, "FleetVariableCreated", map[string]any{
		"actor": actorUserID, "target_type": "variable", "target_id": v.ID, "key": v.Key,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *VariableRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Variable, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	v, err := scanVariable(tx.QueryRow(ctx, variableSelectColumns+` FROM fleet_variables WHERE organization_id = $1 AND id = $2`, organizationID, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrVariableNotFound
		}
		return nil, err
	}
	return v, tx.Commit(ctx)
}

func (r *VariableRepository) ListByComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Variable, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, variableSelectColumns+` FROM fleet_variables WHERE organization_id = $1 AND compose_file_id = $2 ORDER BY key`, organizationID, composeFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var variables []*domain.Variable
	for rows.Next() {
		v, err := scanVariable(rows)
		if err != nil {
			return nil, err
		}
		variables = append(variables, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return variables, tx.Commit(ctx)
}

func (r *VariableRepository) Update(ctx context.Context, actorUserID string, v *domain.Variable) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, v.OrganizationID); err != nil {
		return err
	}

	var value, mountID, path *string
	if v.SecretRef != nil {
		mountID, path = &v.SecretRef.MountID, &v.SecretRef.Path
	} else {
		value = &v.Value
	}
	var fileTargetPath *string
	if v.FileTargetPath != "" {
		fileTargetPath = &v.FileTargetPath
	}

	tag, err := tx.Exec(ctx,
		`UPDATE fleet_variables SET value = $3, secret_mount_id = $4, secret_path = $5, file_target_path = $6
		 WHERE organization_id = $1 AND id = $2`,
		v.OrganizationID, v.ID, value, mountID, path, fileTargetPath,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrVariableNotFound
	}

	if err := outbox.Write(ctx, tx, v.OrganizationID, "FleetVariableUpdated", map[string]any{
		"actor": actorUserID, "target_type": "variable", "target_id": v.ID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *VariableRepository) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM fleet_variables WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrVariableNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetVariableDeleted", map[string]any{
		"actor": actorUserID, "target_type": "variable", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const variableSelectColumns = `SELECT id, organization_id, compose_file_id, key, var_type, value, secret_mount_id, secret_path, file_target_path, created_at`

func scanVariable(row rowScanner) (*domain.Variable, error) {
	var v domain.Variable
	var varType string
	var value, mountID, path, fileTargetPath *string
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ComposeFileID, &v.Key, &varType, &value, &mountID, &path, &fileTargetPath, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	v.VarType = domain.VarType(varType)
	if value != nil {
		v.Value = *value
	}
	if mountID != nil && path != nil {
		v.SecretRef = &domain.SecretReference{MountID: *mountID, Path: *path}
	}
	if fileTargetPath != nil {
		v.FileTargetPath = *fileTargetPath
	}
	return &v, nil
}
