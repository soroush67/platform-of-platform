package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/secrets/application"
	"platform-of-platform/internal/secrets/domain"
)

func newWriteSecretService(vault *fakeVaultClient) (*application.WriteSecretService, *fakeSecretMountRepo, *fakeMembershipChecker, *fakePermissionChecker) {
	repo := newFakeSecretMountRepo()
	membership := newFakeMembershipChecker()
	permChecker := newFakePermissionChecker()
	svc := application.NewWriteSecretService(repo, membership, permChecker, vault, testMasterKey)
	return svc, repo, membership, permChecker
}

func TestWriteSecretService_NonMemberGetsForbidden(t *testing.T) {
	svc, repo, _, _ := newWriteSecretService(newFakeVaultClient("role-1", "secret-1"))
	mount := sealedMount(t, testOrgID, "role-1", "secret-1")
	repo.put(mount)

	err := svc.Execute(context.Background(), application.WriteSecretInput{
		OrganizationID: testOrgID, MountID: mount.ID, RequestingUserID: "stranger",
		Path: "secret/data/fleet/machines/x", Value: "hunter2",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-member, got: %v", err)
	}
}

func TestWriteSecretService_WithoutOrganizationManageGetsForbidden(t *testing.T) {
	svc, repo, membership, _ := newWriteSecretService(newFakeVaultClient("role-1", "secret-1"))
	membership.add(testOrgID, "member-1")
	mount := sealedMount(t, testOrgID, "role-1", "secret-1")
	repo.put(mount)

	err := svc.Execute(context.Background(), application.WriteSecretInput{
		OrganizationID: testOrgID, MountID: mount.ID, RequestingUserID: "member-1",
		Path: "secret/data/fleet/machines/x", Value: "hunter2",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestWriteSecretService_WritesThroughToVault(t *testing.T) {
	vault := newFakeVaultClient("role-1", "the-real-secret-id")
	svc, repo, membership, permChecker := newWriteSecretService(vault)
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	mount := sealedMount(t, testOrgID, "role-1", "the-real-secret-id")
	repo.put(mount)

	if err := svc.Execute(context.Background(), application.WriteSecretInput{
		OrganizationID: testOrgID, MountID: mount.ID, RequestingUserID: "member-1",
		Path: "secret/data/fleet/machines/web-1", Value: "hunter2",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, err := vault.ReadSecret(context.Background(), mount.Address, "role-1", "the-real-secret-id", "secret/data/fleet/machines/web-1")
	if err != nil {
		t.Fatalf("ReadSecret after write: %v", err)
	}
	if got != "hunter2" {
		t.Errorf("expected the written value to read back, got %q", got)
	}
}

func TestWriteSecretService_RequiresPathAndValue(t *testing.T) {
	svc, repo, membership, permChecker := newWriteSecretService(newFakeVaultClient("role-1", "secret-1"))
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")
	mount := sealedMount(t, testOrgID, "role-1", "secret-1")
	repo.put(mount)

	var validationErr *domain.ValidationError
	err := svc.Execute(context.Background(), application.WriteSecretInput{
		OrganizationID: testOrgID, MountID: mount.ID, RequestingUserID: "member-1",
		Path: "", Value: "hunter2",
	})
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an empty path, got: %v", err)
	}
}
