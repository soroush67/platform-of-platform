package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/platform/envelope"
	"platform-of-platform/internal/secrets/application"
	"platform-of-platform/internal/secrets/domain"
)

func sealedMount(t *testing.T, orgID, roleID, secretID string) *domain.SecretMount {
	t.Helper()
	sealed, err := envelope.Seal(testMasterKey, []byte(secretID))
	if err != nil {
		t.Fatalf("envelope.Seal: %v", err)
	}
	mount, err := domain.NewSecretMount(orgID, "primary", domain.BackendTypeVault, "http://vault:8200", roleID, sealed.Ciphertext, sealed.Nonce, sealed.Salt)
	if err != nil {
		t.Fatalf("NewSecretMount: %v", err)
	}
	return mount
}

func newTestConnectionService(vault *fakeVaultClient) (*application.TestConnectionService, *fakeSecretMountRepo, *fakeMembershipChecker, *fakePermissionChecker) {
	repo := newFakeSecretMountRepo()
	membership := newFakeMembershipChecker()
	permChecker := newFakePermissionChecker()
	svc := application.NewTestConnectionService(repo, membership, permChecker, vault, testMasterKey)
	return svc, repo, membership, permChecker
}

func TestTestConnectionService_NonMemberGetsForbidden(t *testing.T) {
	svc, repo, _, _ := newTestConnectionService(newFakeVaultClient("role-1", "secret-1"))
	mount := sealedMount(t, testOrgID, "role-1", "secret-1")
	repo.put(mount)

	err := svc.Execute(context.Background(), testOrgID, mount.ID, "stranger")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-member, got: %v", err)
	}
}

func TestTestConnectionService_UnknownMountGetsNotFound(t *testing.T) {
	svc, _, membership, permChecker := newTestConnectionService(newFakeVaultClient("role-1", "secret-1"))
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")

	err := svc.Execute(context.Background(), testOrgID, "nonexistent-mount", "member-1")
	if !errors.Is(err, domain.ErrSecretMountNotFound) {
		t.Fatalf("expected ErrSecretMountNotFound, got: %v", err)
	}
}

func TestTestConnectionService_DecryptsAndAuthenticatesWithTheRealSecretID(t *testing.T) {
	svc, repo, membership, permChecker := newTestConnectionService(newFakeVaultClient("role-1", "the-real-secret-id"))
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	mount := sealedMount(t, testOrgID, "role-1", "the-real-secret-id")
	repo.put(mount)

	if err := svc.Execute(context.Background(), testOrgID, mount.ID, "member-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestTestConnectionService_WrongSealedSecretIDFailsAuth(t *testing.T) {
	svc, repo, membership, permChecker := newTestConnectionService(newFakeVaultClient("role-1", "the-real-secret-id"))
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	// Sealed a *different* secret_id than what the fake Vault expects -
	// proves this service really does decrypt-then-authenticate, not
	// just return success unconditionally.
	mount := sealedMount(t, testOrgID, "role-1", "wrong-secret-id")
	repo.put(mount)

	if err := svc.Execute(context.Background(), testOrgID, mount.ID, "member-1"); err == nil {
		t.Fatal("expected an auth failure for a mount whose real secret_id doesn't match, got nil")
	}
}
