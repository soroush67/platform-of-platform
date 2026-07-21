package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ConnectionStatus string

const (
	ConnectionStatusUnknown     ConnectionStatus = "unknown"
	ConnectionStatusOnline      ConnectionStatus = "online"
	ConnectionStatusUnreachable ConnectionStatus = "unreachable"
)

type DockerStatus string

const (
	DockerStatusUnknown DockerStatus = "unknown"
	DockerStatusOK      DockerStatus = "ok"
	DockerStatusMissing DockerStatus = "missing"
	DockerStatusError   DockerStatus = "error"
)

// CredentialType is the closed set of SSH auth methods a Machine's
// CredentialRef can resolve to - matching the ported Python product's
// own two real types (it also has a third, gitlab_token, which belongs
// to a Credential that isn't ported in this phase at all).
type CredentialType string

const (
	CredentialTypeSSHKey      CredentialType = "ssh_key"
	CredentialTypeSSHPassword CredentialType = "ssh_password"
)

func (c CredentialType) Valid() bool {
	return c == CredentialTypeSSHKey || c == CredentialTypeSSHPassword
}

// CredentialStorage is the closed set of where a Machine's own SSH
// secret actually lives - "vault" resolves live via CredentialRef at
// connect time (the original, still-default design); "local" seals the
// secret directly into this Machine's own row (EncryptedCredential/
// CredentialNonce/CredentialSalt below) via internal/platform/envelope,
// the same AES-GCM/BLAKE2b scheme SecretMount's own EncryptedSecretID
// already uses - added so a Machine can be created and connected to
// with zero live Vault dependency, operator's own explicit choice after
// hitting a genuinely broken SecretMount (wrong address/role_id) that
// made the Vault-only path a hard blocker.
type CredentialStorage string

const (
	CredentialStorageVault CredentialStorage = "vault"
	CredentialStorageLocal CredentialStorage = "local"
)

func (c CredentialStorage) Valid() bool {
	return c == CredentialStorageVault || c == CredentialStorageLocal
}

// SecretReference is Fleet's own local copy of the value object
// variables/domain.SecretReference already proved out - Fleet never
// imports variables/domain or secrets/domain directly (this codebase's
// no-cross-context-import rule), it declares the shape it needs and a
// SecretMountChecker/SecretResolver port (application/ports.go)
// validates/resolves it against the real secrets/domain.SecretMount
// this MountID points at. Resolved live at SSH-connect time via
// SecretResolver - never persisted as plaintext anywhere in this
// context's own tables (migrations/0019_fleet.up.sql's machines table
// only ever stores mount_id/path, matching variables.secret_mount_id's
// own no-FK, application-validated posture).
type SecretReference struct {
	MountID string
	Path    string
}

// Machine is the aggregate root for a real, deployable remote target -
// its own bounded context's core resource. DeployBasePath is the parent
// directory on the target host a ComposeFile gets written under
// (application/deploy_executor.go appends a deterministic per-
// ComposeFile subdirectory - no separate deploy-target-override entity
// exists in this phase, unlike the Python product's own
// ComposeFileDeployment junction table, since ComposeFile names are
// already unique per Organization).
type Machine struct {
	ID                string
	OrganizationID    string
	Name              string
	Host              string
	SSHPort           int
	SSHUser           string
	CredentialType    CredentialType
	CredentialStorage CredentialStorage
	// CredentialRef is meaningful only when CredentialStorage is
	// CredentialStorageVault.
	CredentialRef SecretReference
	// EncryptedCredential/CredentialNonce/CredentialSalt are meaningful
	// only when CredentialStorage is CredentialStorageLocal - the same
	// three-piece envelope.Sealed shape SecretMount's own
	// EncryptedSecretID/SecretIDNonce/SecretIDSalt already uses.
	EncryptedCredential []byte
	CredentialNonce     []byte
	CredentialSalt      []byte
	DeployBasePath      string
	ConnectionStatus    ConnectionStatus
	DockerStatus        DockerStatus
	LastCheckedAt       *time.Time
	Archived            bool
	CreatedAt           time.Time
}

func NewMachine(organizationID, name, host string, sshPort int, sshUser string, credentialType CredentialType, credentialRef SecretReference, deployBasePath string) (*Machine, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if host == "" {
		return nil, &ValidationError{Message: "host is required"}
	}
	if sshPort < 1 || sshPort > 65535 {
		return nil, &ValidationError{Message: "ssh_port must be between 1 and 65535"}
	}
	if sshUser == "" {
		return nil, &ValidationError{Message: "ssh_user is required"}
	}
	if !credentialType.Valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("credential_type %q must be one of ssh_key, ssh_password", credentialType)}
	}
	if credentialRef.MountID == "" || credentialRef.Path == "" {
		return nil, &ValidationError{Message: "credential mount_id and path are required"}
	}
	if deployBasePath == "" {
		return nil, &ValidationError{Message: "deploy_base_path is required"}
	}

	return &Machine{
		ID:                uuid.NewString(),
		OrganizationID:    organizationID,
		Name:              name,
		Host:              host,
		SSHPort:           sshPort,
		SSHUser:           sshUser,
		CredentialType:    credentialType,
		CredentialStorage: CredentialStorageVault,
		CredentialRef:     credentialRef,
		DeployBasePath:    deployBasePath,
		ConnectionStatus:  ConnectionStatusUnknown,
		DockerStatus:      DockerStatusUnknown,
		CreatedAt:         time.Now().UTC(),
	}, nil
}

// NewMachineWithLocalCredential is NewMachine's sibling for
// CredentialStorageLocal - the caller (CreateMachineService) has
// already sealed the plaintext secret via internal/platform/envelope.Seal
// before calling this; this constructor never sees plaintext, mirroring
// domain.NewSecretMount's own "sealed bytes are constructor inputs" shape.
func NewMachineWithLocalCredential(organizationID, name, host string, sshPort int, sshUser string, credentialType CredentialType, encryptedCredential, credentialNonce, credentialSalt []byte, deployBasePath string) (*Machine, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if host == "" {
		return nil, &ValidationError{Message: "host is required"}
	}
	if sshPort < 1 || sshPort > 65535 {
		return nil, &ValidationError{Message: "ssh_port must be between 1 and 65535"}
	}
	if sshUser == "" {
		return nil, &ValidationError{Message: "ssh_user is required"}
	}
	if !credentialType.Valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("credential_type %q must be one of ssh_key, ssh_password", credentialType)}
	}
	if len(encryptedCredential) == 0 || len(credentialNonce) == 0 || len(credentialSalt) == 0 {
		return nil, &ValidationError{Message: "encrypted credential, nonce, and salt are all required"}
	}
	if deployBasePath == "" {
		return nil, &ValidationError{Message: "deploy_base_path is required"}
	}

	return &Machine{
		ID:                  uuid.NewString(),
		OrganizationID:      organizationID,
		Name:                name,
		Host:                host,
		SSHPort:             sshPort,
		SSHUser:             sshUser,
		CredentialType:      credentialType,
		CredentialStorage:   CredentialStorageLocal,
		EncryptedCredential: encryptedCredential,
		CredentialNonce:     credentialNonce,
		CredentialSalt:      credentialSalt,
		DeployBasePath:      deployBasePath,
		ConnectionStatus:    ConnectionStatusUnknown,
		DockerStatus:        DockerStatusUnknown,
		CreatedAt:           time.Now().UTC(),
	}, nil
}

// RecordConnectionCheck is what TestMachineConnectionService/
// CheckMachineConnectionService's real SSH probe result gets applied
// through - a plain field mutation, matching this codebase's own
// "domain methods shape the aggregate, the application layer decides
// when to call them" split (e.g. Run.MarkFailed elsewhere).
func (m *Machine) RecordConnectionCheck(status ConnectionStatus, docker DockerStatus, at time.Time) {
	m.ConnectionStatus = status
	m.DockerStatus = docker
	m.LastCheckedAt = &at
}
