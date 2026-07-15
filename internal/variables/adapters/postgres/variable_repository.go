package postgres

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/variables/domain"
)

type VariableRepository struct {
	pool *pgxpool.Pool
}

func NewVariableRepository(pool *pgxpool.Pool) *VariableRepository {
	return &VariableRepository{pool: pool}
}

func (r *VariableRepository) Create(ctx context.Context, v *domain.Variable) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, v.OrganizationID); err != nil {
		return err
	}

	value, mountID, path := valueOrSecretRefColumns(v)

	_, err = tx.Exec(ctx,
		`INSERT INTO variables (id, organization_id, scope_type, scope_id, key, category, sensitivity, value, secret_mount_id, secret_path, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		v.ID, v.OrganizationID, string(v.ScopeType), v.ScopeID, v.Key, string(v.Category), string(v.Sensitivity), value, mountID, path, v.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// valueOrSecretRefColumns implements Value XOR SecretRef
// (migrations/0018_secrets.up.sql's own CHECK constraint) at the Go
// side of the insert - exactly one of (value) or (secret_mount_id,
// secret_path) is ever non-NULL.
func valueOrSecretRefColumns(v *domain.Variable) (value, mountID, path sql.NullString) {
	if v.SecretRef != nil {
		return sql.NullString{}, sql.NullString{String: v.SecretRef.MountID, Valid: true}, sql.NullString{String: v.SecretRef.Path, Valid: true}
	}
	return sql.NullString{String: v.Value, Valid: true}, sql.NullString{}, sql.NullString{}
}

func (r *VariableRepository) GetByScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID, key string) (*domain.Variable, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	v, err := scanVariable(tx.QueryRow(ctx,
		`SELECT id, organization_id, scope_type, scope_id, key, category, sensitivity, value, secret_mount_id, secret_path, created_at
		 FROM variables WHERE organization_id = $1 AND scope_type = $2 AND scope_id = $3 AND key = $4`,
		organizationID, string(scopeType), scopeID, key,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrVariableNotFound
		}
		return nil, err
	}

	return v, tx.Commit(ctx)
}

func (r *VariableRepository) ListByScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID string) ([]*domain.Variable, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, scope_type, scope_id, key, category, sensitivity, value, secret_mount_id, secret_path, created_at
		 FROM variables WHERE organization_id = $1 AND scope_type = $2 AND scope_id = $3 ORDER BY key`,
		organizationID, string(scopeType), scopeID,
	)
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

// GetByID is what UpdateVariableService/DeleteVariableService use to
// look a Variable up by its own id (the URL's {variableID}, not a
// scope+key triple) - the resolution cascade (GetByScope) and the
// direct-CRUD paths (this method) are genuinely different lookup shapes,
// so this is a real, separate query, not a thin wrapper over GetByScope.
func (r *VariableRepository) GetByID(ctx context.Context, organizationID, variableID string) (*domain.Variable, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	v, err := scanVariable(tx.QueryRow(ctx,
		`SELECT id, organization_id, scope_type, scope_id, key, category, sensitivity, value, secret_mount_id, secret_path, created_at
		 FROM variables WHERE organization_id = $1 AND id = $2`,
		organizationID, variableID,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrVariableNotFound
		}
		return nil, err
	}

	return v, tx.Commit(ctx)
}

// Update changes Value/Category/Sensitivity in place - Key and ScopeType/
// ScopeID are deliberately immutable (docs/architecture/05-database.md's
// own UNIQUE(scope_type, scope_id, key) is this Variable's actual
// identity; changing either is "delete this one, create a different
// one," not an update to the same resource).
func (r *VariableRepository) Update(ctx context.Context, v *domain.Variable) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, v.OrganizationID); err != nil {
		return err
	}

	// Always writes a literal value and clears any secret_ref -
	// UpdateVariableInput has no secret_ref field of its own (see the
	// application layer's own comment on why), so an Update always
	// leaves the row in the plain-Value branch of the XOR.
	_, err = tx.Exec(ctx,
		`UPDATE variables SET value = $3, secret_mount_id = NULL, secret_path = NULL, category = $4, sensitivity = $5 WHERE organization_id = $1 AND id = $2`,
		v.OrganizationID, v.ID, v.Value, string(v.Category), string(v.Sensitivity),
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *VariableRepository) Delete(ctx context.Context, organizationID, variableID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM variables WHERE organization_id = $1 AND id = $2`, organizationID, variableID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVariable(row rowScanner) (*domain.Variable, error) {
	var v domain.Variable
	var value, mountID, path sql.NullString
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ScopeType, &v.ScopeID, &v.Key, &v.Category, &v.Sensitivity, &value, &mountID, &path, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	if mountID.Valid {
		v.SecretRef = &domain.SecretReference{MountID: mountID.String, Path: path.String}
	} else {
		v.Value = value.String
	}
	return &v, nil
}
