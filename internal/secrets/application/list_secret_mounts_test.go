package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/secrets/application"
	"platform-of-platform/internal/secrets/domain"
)

func TestListSecretMountsService_NonMemberGetsForbidden(t *testing.T) {
	repo := newFakeSecretMountRepo()
	membership := newFakeMembershipChecker()
	svc := application.NewListSecretMountsService(repo, membership)

	_, err := svc.Execute(context.Background(), testOrgID, "stranger")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-member, got: %v", err)
	}
}

func TestListSecretMountsService_ReturnsOnlyThisOrganizationsMounts(t *testing.T) {
	repo := newFakeSecretMountRepo()
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	svc := application.NewListSecretMountsService(repo, membership)

	mine, err := domain.NewSecretMount(testOrgID, "mine", domain.BackendTypeVault, "http://vault:8200", "role-1", []byte("ct"), []byte("nonce"), []byte("salt"))
	if err != nil {
		t.Fatalf("NewSecretMount: %v", err)
	}
	repo.put(mine)
	other, err := domain.NewSecretMount("org-2", "other", domain.BackendTypeVault, "http://vault:8200", "role-2", []byte("ct"), []byte("nonce"), []byte("salt"))
	if err != nil {
		t.Fatalf("NewSecretMount: %v", err)
	}
	repo.put(other)

	mounts, err := svc.Execute(context.Background(), testOrgID, "member-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mounts) != 1 || mounts[0].ID != mine.ID {
		t.Errorf("expected exactly this organization's own mount, got: %+v", mounts)
	}
}
