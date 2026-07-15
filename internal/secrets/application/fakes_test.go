package application_test

import (
	"context"
	"errors"
	"sync"

	"platform-of-platform/internal/secrets/domain"
)

var (
	errAuthFailed     = errors.New("fake vault: approle login failed: invalid role_id/secret_id")
	errSecretNotFound = errors.New("fake vault: no secret found at path")
)

type fakeSecretMountRepo struct {
	mu     sync.Mutex
	mounts map[string]*domain.SecretMount
}

func newFakeSecretMountRepo() *fakeSecretMountRepo {
	return &fakeSecretMountRepo{mounts: map[string]*domain.SecretMount{}}
}

func (f *fakeSecretMountRepo) Create(ctx context.Context, m *domain.SecretMount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *m
	f.mounts[m.ID] = &cp
	return nil
}

func (f *fakeSecretMountRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.SecretMount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.mounts[id]
	if !ok || m.OrganizationID != organizationID {
		return nil, domain.ErrSecretMountNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeSecretMountRepo) ListForOrganization(ctx context.Context, organizationID string) ([]*domain.SecretMount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.SecretMount
	for _, m := range f.mounts {
		if m.OrganizationID == organizationID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeSecretMountRepo) put(m *domain.SecretMount) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *m
	f.mounts[m.ID] = &cp
}

type fakeMembershipChecker struct {
	mu      sync.Mutex
	members map[string]bool
}

func newFakeMembershipChecker() *fakeMembershipChecker {
	return &fakeMembershipChecker{members: map[string]bool{}}
}

func (f *fakeMembershipChecker) IsMember(ctx context.Context, organizationID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[organizationID+"|"+userID], nil
}

func (f *fakeMembershipChecker) add(orgID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[orgID+"|"+userID] = true
}

type fakePermissionChecker struct {
	mu    sync.Mutex
	perms map[string]bool
}

func newFakePermissionChecker() *fakePermissionChecker {
	return &fakePermissionChecker{perms: map[string]bool{}}
}

func (f *fakePermissionChecker) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perms[organizationID+"|"+userID+"|"+permission], nil
}

func (f *fakePermissionChecker) grant(orgID, userID, permission string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.perms[orgID+"|"+userID+"|"+permission] = true
}

// fakeVaultClient is a real in-memory stand-in for the Vault Go SDK
// adapter - the only place this whole context's test suite doesn't talk
// to a real backend, since VaultClient's own real implementation
// (internal/secrets/adapters/vault) genuinely requires a live Vault
// server, out of scope for this package's fast, hermetic unit tests
// (the real Vault path gets its own real end-to-end verification
// against docker-compose's dev-mode Vault service separately).
type fakeVaultClient struct {
	mu            sync.Mutex
	validRoleID   string
	validSecretID string
	secretsByPath map[string]string
	testConnErr   error
	readSecretErr error
}

func newFakeVaultClient(validRoleID, validSecretID string) *fakeVaultClient {
	return &fakeVaultClient{validRoleID: validRoleID, validSecretID: validSecretID, secretsByPath: map[string]string{}}
}

func (f *fakeVaultClient) TestConnection(ctx context.Context, address, roleID, secretID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.testConnErr != nil {
		return f.testConnErr
	}
	if roleID != f.validRoleID || secretID != f.validSecretID {
		return errAuthFailed
	}
	return nil
}

func (f *fakeVaultClient) ReadSecret(ctx context.Context, address, roleID, secretID, path string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readSecretErr != nil {
		return "", f.readSecretErr
	}
	if roleID != f.validRoleID || secretID != f.validSecretID {
		return "", errAuthFailed
	}
	value, ok := f.secretsByPath[path]
	if !ok {
		return "", errSecretNotFound
	}
	return value, nil
}

func (f *fakeVaultClient) putSecret(path, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secretsByPath[path] = value
}
