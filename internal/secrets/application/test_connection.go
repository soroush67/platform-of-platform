package application

import (
	"context"

	"platform-of-platform/internal/platform/envelope"
	"platform-of-platform/internal/secrets/domain"
)

// TestConnectionService implements
// `POST /secret-mounts/{mount}/test-connection` (docs/architecture/
// 11-module-secrets-state.md §1's own "verifies the stored credential
// can actually authenticate... without revealing any secret content") -
// decrypts the mount's own sealed secret_id just long enough to attempt
// a real AppRole login against the real backend, then discards it;
// Execute's own return is just (error or nil), nothing about the
// credential or the backend's response ever flows back to the caller.
type TestConnectionService struct {
	repo        SecretMountRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	vault       VaultClient
	masterKey   []byte
}

func NewTestConnectionService(repo SecretMountRepository, membership MembershipChecker, permChecker PermissionChecker, vault VaultClient, masterKey []byte) *TestConnectionService {
	return &TestConnectionService{repo: repo, membership: membership, permChecker: permChecker, vault: vault, masterKey: masterKey}
}

func (s *TestConnectionService) Execute(ctx context.Context, organizationID, mountID, requestingUserID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}

	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	mount, err := s.repo.GetByID(ctx, organizationID, mountID)
	if err != nil {
		return err
	}

	secretID, err := decryptSecretID(s.masterKey, mount)
	if err != nil {
		return err
	}

	switch mount.BackendType {
	case domain.BackendTypeVault:
		return s.vault.TestConnection(ctx, mount.Address, mount.RoleID, secretID)
	default:
		// Unreachable in practice - CreateSecretMountService already
		// rejects every backend_type but vault before a row can exist -
		// kept as a real, explicit case rather than falling through
		// silently if that constraint is ever loosened later.
		return &domain.ValidationError{Message: "backend_type " + string(mount.BackendType) + " is not yet implemented"}
	}
}

// decryptSecretID is shared by TestConnectionService and Variables'
// own SecretResolver adapter (both need the real secret_id just long
// enough to authenticate to the mount's backend).
func decryptSecretID(masterKey []byte, mount *domain.SecretMount) (string, error) {
	plaintext, err := envelope.Open(masterKey, &envelope.Sealed{
		Ciphertext: mount.EncryptedSecretID,
		Nonce:      mount.SecretIDNonce,
		Salt:       mount.SecretIDSalt,
	})
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
