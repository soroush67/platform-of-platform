package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/outbox"
)

type MachineRepository struct {
	pool *pgxpool.Pool
}

func NewMachineRepository(pool *pgxpool.Pool) *MachineRepository {
	return &MachineRepository{pool: pool}
}

func (r *MachineRepository) Create(ctx context.Context, actorUserID string, m *domain.Machine) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, m.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO machines (id, organization_id, name, host, ssh_port, ssh_user, credential_type, credential_mount_id, credential_path, deploy_base_path, connection_status, docker_status, archived, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		m.ID, m.OrganizationID, m.Name, m.Host, m.SSHPort, m.SSHUser, string(m.CredentialType),
		m.CredentialRef.MountID, m.CredentialRef.Path, m.DeployBasePath,
		string(m.ConnectionStatus), string(m.DockerStatus), m.Archived, m.CreatedAt,
	)
	if err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, m.OrganizationID, "FleetMachineCreated", map[string]any{
		"actor": actorUserID, "target_type": "machine", "target_id": m.ID, "name": m.Name, "host": m.Host,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *MachineRepository) GetByID(ctx context.Context, organizationID, id string) (*domain.Machine, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	m, err := scanMachine(tx.QueryRow(ctx, machineSelectColumns+` FROM machines WHERE organization_id = $1 AND id = $2`, organizationID, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrMachineNotFound
		}
		return nil, err
	}
	return m, tx.Commit(ctx)
}

func (r *MachineRepository) ListByOrganization(ctx context.Context, organizationID string, includeArchived bool) ([]*domain.Machine, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	query := machineSelectColumns + ` FROM machines WHERE organization_id = $1`
	if !includeArchived {
		query += ` AND archived = false`
	}
	query += ` ORDER BY created_at`

	rows, err := tx.Query(ctx, query, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []*domain.Machine
	for rows.Next() {
		m, err := scanMachine(rows)
		if err != nil {
			return nil, err
		}
		machines = append(machines, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return machines, tx.Commit(ctx)
}

func (r *MachineRepository) Update(ctx context.Context, actorUserID string, m *domain.Machine) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, m.OrganizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE machines SET ssh_user = $3, credential_type = $4, credential_mount_id = $5, credential_path = $6,
		 deploy_base_path = $7, connection_status = $8, docker_status = $9, last_checked_at = $10
		 WHERE organization_id = $1 AND id = $2`,
		m.OrganizationID, m.ID, m.SSHUser, string(m.CredentialType), m.CredentialRef.MountID, m.CredentialRef.Path,
		m.DeployBasePath, string(m.ConnectionStatus), string(m.DockerStatus), m.LastCheckedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrMachineNotFound
	}

	if err := outbox.Write(ctx, tx, m.OrganizationID, "FleetMachineUpdated", map[string]any{
		"actor": actorUserID, "target_type": "machine", "target_id": m.ID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Delete returns domain.ErrMachineHasHistory on a real 23503 foreign-key
// violation (real Operation rows reference this machine) -
// ArchiveMachineService catches that and falls back to Archive below,
// same delete-or-archive-fallback behavior as the Python original.
func (r *MachineRepository) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM machines WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.ErrMachineHasHistory
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrMachineNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetMachineDeleted", map[string]any{
		"actor": actorUserID, "target_type": "machine", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *MachineRepository) Archive(ctx context.Context, actorUserID, organizationID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `UPDATE machines SET archived = true WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrMachineNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetMachineArchived", map[string]any{
		"actor": actorUserID, "target_type": "machine", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const machineSelectColumns = `SELECT id, organization_id, name, host, ssh_port, ssh_user, credential_type, credential_mount_id, credential_path, deploy_base_path, connection_status, docker_status, last_checked_at, archived, created_at`

func scanMachine(row rowScanner) (*domain.Machine, error) {
	var m domain.Machine
	var credentialType, connStatus, dockerStatus string
	err := row.Scan(&m.ID, &m.OrganizationID, &m.Name, &m.Host, &m.SSHPort, &m.SSHUser, &credentialType,
		&m.CredentialRef.MountID, &m.CredentialRef.Path, &m.DeployBasePath, &connStatus, &dockerStatus,
		&m.LastCheckedAt, &m.Archived, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.CredentialType = domain.CredentialType(credentialType)
	m.ConnectionStatus = domain.ConnectionStatus(connStatus)
	m.DockerStatus = domain.DockerStatus(dockerStatus)
	return &m, nil
}
