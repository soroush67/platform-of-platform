package application

import (
	"context"

	"platform-of-platform/internal/secrets/domain"
)

// ResolveSecretService is what Variables' own SecretResolver port
// (internal/variables/application/ports.go) is wired to in main.go -
// looks up the mount, decrypts its bootstrap credential just long
// enough to authenticate, and reads the real secret content live from
// the real backend on every call. Nothing about a resolved secret's
// value is ever cached or persisted anywhere in this codebase - a
// Variable with a SecretRef genuinely never has its real value written
// to Postgres, closing the actual gap this whole context exists to
// close.
//
// Deliberately no membership/permission check of its own:
// ResolveVariableService (Variables' own caller) has already gated on
// the Variable's own scope-membership check before ever reaching this -
// re-checking membership here would need a second, redundant RBAC round
// trip on every single variable resolution, and this method's only
// caller is a fixed, known internal cross-context port, not a public
// HTTP route of its own.
type ResolveSecretService struct {
	repo      SecretMountRepository
	vault     VaultClient
	masterKey []byte
}

func NewResolveSecretService(repo SecretMountRepository, vault VaultClient, masterKey []byte) *ResolveSecretService {
	return &ResolveSecretService{repo: repo, vault: vault, masterKey: masterKey}
}

// ResolveValue matches the exact signature shape Execution's own
// VariableResolver port already mirrors from Variables'
// ResolveVariableService.ResolveValue - same "cross-context port,
// plain strings, no shared type" convention this codebase already
// established.
func (s *ResolveSecretService) ResolveValue(ctx context.Context, organizationID, mountID, path string) (string, error) {
	mount, err := s.repo.GetByID(ctx, organizationID, mountID)
	if err != nil {
		return "", err
	}

	secretID, err := decryptSecretID(s.masterKey, mount)
	if err != nil {
		return "", err
	}

	switch mount.BackendType {
	case domain.BackendTypeVault:
		return s.vault.ReadSecret(ctx, mount.Address, mount.RoleID, secretID, path)
	default:
		return "", &domain.ValidationError{Message: "backend_type " + string(mount.BackendType) + " is not yet implemented"}
	}
}
