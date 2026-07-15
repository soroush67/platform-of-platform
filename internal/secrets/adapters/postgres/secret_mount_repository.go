// Package postgres - see tenancy's identically-named package for why
// "postgres" names the wire protocol, not the actual engine
// (CockroachDB, docs/architecture/05-database.md §0).
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/secrets/domain"
)

type SecretMountRepository struct {
	pool *pgxpool.Pool
}

func NewSecretMountRepository(pool *pgxpool.Pool) *SecretMountRepository {
	return &SecretMountRepository{pool: pool}
}

func (r *SecretMountRepository) Create(ctx context.Context, mount *domain.SecretMount) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, mount.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO secret_mounts (id, organization_id, name, backend_type, address, role_id, encrypted_secret_id, secret_id_nonce, secret_id_salt, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		mount.ID, mount.OrganizationID, mount.Name, string(mount.BackendType), mount.Address, mount.RoleID,
		mount.EncryptedSecretID, mount.SecretIDNonce, mount.SecretIDSalt, mount.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *SecretMountRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.SecretMount, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	m, err := scanMount(tx.QueryRow(ctx,
		`SELECT id, organization_id, name, backend_type, address, role_id, encrypted_secret_id, secret_id_nonce, secret_id_salt, created_at
		 FROM secret_mounts WHERE organization_id = $1 AND id = $2`,
		organizationID, id,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrSecretMountNotFound
		}
		return nil, err
	}

	return m, tx.Commit(ctx)
}

func (r *SecretMountRepository) ListForOrganization(ctx context.Context, organizationID string) ([]*domain.SecretMount, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, name, backend_type, address, role_id, encrypted_secret_id, secret_id_nonce, secret_id_salt, created_at
		 FROM secret_mounts WHERE organization_id = $1 ORDER BY created_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mounts []*domain.SecretMount
	for rows.Next() {
		m, err := scanMount(rows)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return mounts, tx.Commit(ctx)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMount(row rowScanner) (*domain.SecretMount, error) {
	var m domain.SecretMount
	var backendType string
	err := row.Scan(&m.ID, &m.OrganizationID, &m.Name, &backendType, &m.Address, &m.RoleID,
		&m.EncryptedSecretID, &m.SecretIDNonce, &m.SecretIDSalt, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.BackendType = domain.BackendType(backendType)
	return &m, nil
}
