package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/identity/domain"
)

type ServiceAccountRepository struct {
	pool *pgxpool.Pool
}

func NewServiceAccountRepository(pool *pgxpool.Pool) *ServiceAccountRepository {
	return &ServiceAccountRepository{pool: pool}
}

func (r *ServiceAccountRepository) Create(ctx context.Context, sa *domain.ServiceAccount) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, sa.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO service_accounts (id, organization_id, name, description, created_at) VALUES ($1, $2, $3, $4, $5)`,
		sa.ID, sa.OrganizationID, sa.Name, sa.Description, sa.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *ServiceAccountRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.ServiceAccount, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var sa domain.ServiceAccount
	err = tx.QueryRow(ctx,
		`SELECT id, organization_id, name, description, created_at FROM service_accounts WHERE id = $1 AND organization_id = $2`,
		id, organizationID,
	).Scan(&sa.ID, &sa.OrganizationID, &sa.Name, &sa.Description, &sa.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrServiceAccountNotFound
		}
		return nil, err
	}

	return &sa, tx.Commit(ctx)
}

// ServiceAccountExists is RBAC's own ServiceAccountChecker port
// (internal/rbac/application/ports.go) - validates a role-binding
// subject actually resolves to a real ServiceAccount in this org before
// a grant/deny is created for it.
func (r *ServiceAccountRepository) ServiceAccountExists(ctx context.Context, organizationID, serviceAccountID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM service_accounts WHERE id = $1 AND organization_id = $2)`, serviceAccountID, organizationID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, tx.Commit(ctx)
}
