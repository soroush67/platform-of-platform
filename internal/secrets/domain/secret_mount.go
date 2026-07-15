// Package domain holds the Secrets context's pure Go types
// (docs/architecture/11-module-secrets-state.md §1). No crypto here -
// same "pure Go, zero third-party imports" rule already applied to
// every other context's /domain package (e.g. identity/domain's own
// PasswordHash: set by the application layer via a plain string
// setter, never computed in domain) - SecretMount holds the already-
// sealed bytes, CreateSecretMountService is what calls
// internal/platform/envelope.Seal before constructing one.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSecretMountNotFound = errors.New("secret mount not found")
	ErrForbidden           = errors.New("forbidden")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// BackendType is the closed set from docs/architecture/11-module-
// secrets-state.md §1's own REST API shape - modeled fully (matching
// this codebase's own "closed set -> real type" discipline, e.g.
// execution/domain.RunStatus), but only BackendTypeVault is actually
// implemented anywhere in this codebase yet (internal/secrets/adapters/
// vault) - CreateSecretMountService rejects the other three with a
// clear "not yet implemented" validation error rather than silently
// accepting a mount this codebase can never actually connect to.
type BackendType string

const (
	BackendTypeVault             BackendType = "vault"
	BackendTypeAWSSecretsManager BackendType = "aws_secrets_manager"
	BackendTypeAzureKeyVault     BackendType = "azure_keyvault"
	BackendTypeGCPSecretManager  BackendType = "gcp_secret_manager"
)

func (b BackendType) Valid() bool {
	switch b {
	case BackendTypeVault, BackendTypeAWSSecretsManager, BackendTypeAzureKeyVault, BackendTypeGCPSecretManager:
		return true
	}
	return false
}

// SecretMount is the aggregate root - an Organization's own connection
// to a real secret backend, authenticated via Vault's AppRole method
// (role_id + secret_id). The secret_id is the one real credential this
// codebase has "no further backend to defer to" for
// (docs/architecture/11-module-secrets-state.md §1's own framing) - it
// never appears here as a plaintext field, only as the three pieces
// internal/platform/envelope.Seal produces.
type SecretMount struct {
	ID                string
	OrganizationID    string
	Name              string
	BackendType       BackendType
	Address           string
	RoleID            string
	EncryptedSecretID []byte
	SecretIDNonce     []byte
	SecretIDSalt      []byte
	CreatedAt         time.Time
}

func NewSecretMount(organizationID, name string, backendType BackendType, address, roleID string, encryptedSecretID, secretIDNonce, secretIDSalt []byte) (*SecretMount, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if !backendType.Valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("backend_type %q must be one of vault, aws_secrets_manager, azure_keyvault, gcp_secret_manager", backendType)}
	}
	if address == "" {
		return nil, &ValidationError{Message: "address is required"}
	}
	if roleID == "" {
		return nil, &ValidationError{Message: "role_id is required"}
	}
	if len(encryptedSecretID) == 0 || len(secretIDNonce) == 0 || len(secretIDSalt) == 0 {
		return nil, &ValidationError{Message: "secret_id is required"}
	}

	return &SecretMount{
		ID:                uuid.NewString(),
		OrganizationID:    organizationID,
		Name:              name,
		BackendType:       backendType,
		Address:           address,
		RoleID:            roleID,
		EncryptedSecretID: encryptedSecretID,
		SecretIDNonce:     secretIDNonce,
		SecretIDSalt:      secretIDSalt,
		CreatedAt:         time.Now().UTC(),
	}, nil
}
