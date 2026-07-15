package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/secrets/application"
)

func newResolveSecretService(vault *fakeVaultClient) (*application.ResolveSecretService, *fakeSecretMountRepo) {
	repo := newFakeSecretMountRepo()
	svc := application.NewResolveSecretService(repo, vault, testMasterKey)
	return svc, repo
}

func TestResolveSecretService_ReturnsTheRealLiveValue(t *testing.T) {
	vault := newFakeVaultClient("role-1", "the-real-secret-id")
	vault.putSecret("secret/data/database/prod/password", "hunter2")
	svc, repo := newResolveSecretService(vault)
	mount := sealedMount(t, testOrgID, "role-1", "the-real-secret-id")
	repo.put(mount)

	value, err := svc.ResolveValue(context.Background(), testOrgID, mount.ID, "secret/data/database/prod/password")
	if err != nil {
		t.Fatalf("ResolveValue: %v", err)
	}
	if value != "hunter2" {
		t.Errorf("expected the real secret value, got %q", value)
	}
}

func TestResolveSecretService_UnknownMountReturnsError(t *testing.T) {
	svc, _ := newResolveSecretService(newFakeVaultClient("role-1", "secret-1"))

	_, err := svc.ResolveValue(context.Background(), testOrgID, "nonexistent-mount", "secret/data/x")
	if err == nil {
		t.Fatal("expected an error for a nonexistent mount, got nil")
	}
}

func TestResolveSecretService_MissingPathReturnsError(t *testing.T) {
	vault := newFakeVaultClient("role-1", "the-real-secret-id")
	svc, repo := newResolveSecretService(vault)
	mount := sealedMount(t, testOrgID, "role-1", "the-real-secret-id")
	repo.put(mount)

	_, err := svc.ResolveValue(context.Background(), testOrgID, mount.ID, "secret/data/does-not-exist")
	if err == nil {
		t.Fatal("expected an error when the path has no secret, got nil")
	}
	if !errors.Is(err, errSecretNotFound) {
		t.Errorf("expected the fake backend's own not-found error to surface, got: %v", err)
	}
}
