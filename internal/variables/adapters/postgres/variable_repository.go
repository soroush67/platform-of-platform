package postgres

import (
	"context"

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

	_, err = tx.Exec(ctx,
		`INSERT INTO variables (id, organization_id, scope_type, scope_id, key, category, sensitivity, value, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		v.ID, v.OrganizationID, string(v.ScopeType), v.ScopeID, v.Key, string(v.Category), string(v.Sensitivity), v.Value, v.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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
		`SELECT id, organization_id, scope_type, scope_id, key, category, sensitivity, value, created_at
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
		`SELECT id, organization_id, scope_type, scope_id, key, category, sensitivity, value, created_at
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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVariable(row rowScanner) (*domain.Variable, error) {
	var v domain.Variable
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ScopeType, &v.ScopeID, &v.Key, &v.Category, &v.Sensitivity, &v.Value, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
