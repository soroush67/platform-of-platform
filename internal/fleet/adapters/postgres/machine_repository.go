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

	mountID, path := credentialRefColumns(m)
	encryptedCredential, credentialNonce, credentialSalt := localCredentialColumns(m)

	_, err = tx.Exec(ctx,
		`INSERT INTO machines (id, organization_id, name, host, ssh_port, ssh_user, credential_type, credential_storage,
		  credential_mount_id, credential_path, encrypted_credential, credential_nonce, credential_salt,
		  deploy_base_path, connection_status, docker_status, archived, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		m.ID, m.OrganizationID, m.Name, m.Host, m.SSHPort, m.SSHUser, string(m.CredentialType), string(m.CredentialStorage),
		mountID, path, encryptedCredential, credentialNonce, credentialSalt, m.DeployBasePath,
		string(m.ConnectionStatus), string(m.DockerStatus), m.Archived, m.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrMachineNameTaken
		}
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

	mountID, path := credentialRefColumns(m)
	encryptedCredential, credentialNonce, credentialSalt := localCredentialColumns(m)

	tag, err := tx.Exec(ctx,
		`UPDATE machines SET ssh_user = $3, credential_type = $4, credential_storage = $5,
		 credential_mount_id = $6, credential_path = $7,
		 encrypted_credential = $8, credential_nonce = $9, credential_salt = $10,
		 deploy_base_path = $11, connection_status = $12, docker_status = $13, last_checked_at = $14
		 WHERE organization_id = $1 AND id = $2`,
		m.OrganizationID, m.ID, m.SSHUser, string(m.CredentialType), string(m.CredentialStorage),
		mountID, path, encryptedCredential, credentialNonce, credentialSalt,
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

func (r *MachineRepository) Unarchive(ctx context.Context, actorUserID, organizationID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `UPDATE machines SET archived = false WHERE organization_id = $1 AND id = $2`, organizationID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrMachineNotFound
	}

	if err := outbox.Write(ctx, tx, organizationID, "FleetMachineUnarchived", map[string]any{
		"actor": actorUserID, "target_type": "machine", "target_id": id,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const machineSelectColumns = `SELECT id, organization_id, name, host, ssh_port, ssh_user, credential_type, credential_storage,
	credential_mount_id, credential_path, encrypted_credential, credential_nonce, credential_salt,
	deploy_base_path, connection_status, docker_status, last_checked_at, archived, created_at`

func scanMachine(row rowScanner) (*domain.Machine, error) {
	var m domain.Machine
	var credentialType, credentialStorage, connStatus, dockerStatus string
	var mountID, path *string
	err := row.Scan(&m.ID, &m.OrganizationID, &m.Name, &m.Host, &m.SSHPort, &m.SSHUser, &credentialType, &credentialStorage,
		&mountID, &path, &m.EncryptedCredential, &m.CredentialNonce, &m.CredentialSalt,
		&m.DeployBasePath, &connStatus, &dockerStatus,
		&m.LastCheckedAt, &m.Archived, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.CredentialType = domain.CredentialType(credentialType)
	m.CredentialStorage = domain.CredentialStorage(credentialStorage)
	if mountID != nil {
		m.CredentialRef.MountID = *mountID
	}
	if path != nil {
		m.CredentialRef.Path = *path
	}
	m.ConnectionStatus = domain.ConnectionStatus(connStatus)
	m.DockerStatus = domain.DockerStatus(dockerStatus)
	return &m, nil
}

// credentialRefColumns/localCredentialColumns build the nullable column
// values Create/Update both need - exactly one shape is ever non-nil,
// matching migrations/0025_machine_local_credential.up.sql's own
// machines_credential_shape CHECK constraint (a bug here would be
// caught by that constraint at INSERT/UPDATE time, not silently accepted).
func credentialRefColumns(m *domain.Machine) (mountID, path *string) {
	if m.CredentialStorage != domain.CredentialStorageVault {
		return nil, nil
	}
	return &m.CredentialRef.MountID, &m.CredentialRef.Path
}

func localCredentialColumns(m *domain.Machine) (encryptedCredential, credentialNonce, credentialSalt []byte) {
	if m.CredentialStorage != domain.CredentialStorageLocal {
		return nil, nil, nil
	}
	return m.EncryptedCredential, m.CredentialNonce, m.CredentialSalt
}
