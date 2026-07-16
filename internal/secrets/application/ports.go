package application

import (
	"context"

	"platform-of-platform/internal/secrets/domain"
)

type SecretMountRepository interface {
	Create(ctx context.Context, mount *domain.SecretMount) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.SecretMount, error)
	ListForOrganization(ctx context.Context, organizationID string) ([]*domain.SecretMount, error)
}

// MembershipChecker/PermissionChecker - this context's own copies of
// the same port shape every other context declares locally
// (docs/architecture/18-backend-structure.md §3's dependency-inversion
// rule).
type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// VaultClient is this context's own port into the real Vault Go SDK
// adapter (internal/secrets/adapters/vault) - a real AppRole login
// followed by either a content-free connectivity check (TestConnection,
// docs/architecture/11-module-secrets-state.md §1's own
// "verifies the stored credential can actually authenticate... without
// revealing any secret content") or a real KV v2 read (ReadSecret, what
// Variables' own SecretResolver port ultimately calls through to).
// address/roleID/secretID are passed per call, not held by the client -
// a SecretMount can point at a different Vault instance per
// Organization, so there's no single "the" Vault connection to cache.
type VaultClient interface {
	TestConnection(ctx context.Context, address, roleID, secretID string) error
	ReadSecret(ctx context.Context, address, roleID, secretID, path string) (string, error)
	// WriteSecret writes value as a KV v2 secret at path - lets a caller
	// (e.g. Fleet's Add Machine form) store a credential directly through
	// this platform rather than requiring an out-of-band `vault kv put`
	// first, closing the real UX gap the read-only version of this port
	// left.
	WriteSecret(ctx context.Context, address, roleID, secretID, path, value string) error
}
