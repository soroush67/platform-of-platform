package application_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/secrets/application"
	"platform-of-platform/internal/secrets/domain"
)

const testOrgID = "org-1"

var testMasterKey = bytes.Repeat([]byte{0x42}, 32)

func newCreateSecretMountService() (*application.CreateSecretMountService, *fakeSecretMountRepo, *fakeMembershipChecker, *fakePermissionChecker) {
	repo := newFakeSecretMountRepo()
	membership := newFakeMembershipChecker()
	permChecker := newFakePermissionChecker()
	svc := application.NewCreateSecretMountService(repo, membership, permChecker, testMasterKey)
	return svc, repo, membership, permChecker
}

func TestCreateSecretMountService_NonMemberGetsForbidden(t *testing.T) {
	svc, _, _, _ := newCreateSecretMountService()

	_, err := svc.Execute(context.Background(), application.CreateSecretMountInput{
		OrganizationID: testOrgID, RequestingUserID: "stranger", Name: "primary", BackendType: "vault",
		Address: "http://vault:8200", RoleID: "role-1", SecretID: "secret-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-member, got: %v", err)
	}
}

func TestCreateSecretMountService_MemberWithoutPermissionGetsForbidden(t *testing.T) {
	svc, _, membership, _ := newCreateSecretMountService()
	membership.add(testOrgID, "member-1")

	_, err := svc.Execute(context.Background(), application.CreateSecretMountInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", Name: "primary", BackendType: "vault",
		Address: "http://vault:8200", RoleID: "role-1", SecretID: "secret-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestCreateSecretMountService_RejectsNonVaultBackend(t *testing.T) {
	svc, _, membership, permChecker := newCreateSecretMountService()
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")

	_, err := svc.Execute(context.Background(), application.CreateSecretMountInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", Name: "primary", BackendType: "aws_secrets_manager",
		Address: "http://aws", RoleID: "role-1", SecretID: "secret-1",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unimplemented backend_type, got: %v", err)
	}
}

func TestCreateSecretMountService_RejectsEmptySecretID(t *testing.T) {
	svc, _, membership, permChecker := newCreateSecretMountService()
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")

	_, err := svc.Execute(context.Background(), application.CreateSecretMountInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", Name: "primary", BackendType: "vault",
		Address: "http://vault:8200", RoleID: "role-1", SecretID: "",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an empty secret_id, got: %v", err)
	}
}

func TestCreateSecretMountService_SucceedsAndNeverPersistsThePlaintextSecretID(t *testing.T) {
	svc, repo, membership, permChecker := newCreateSecretMountService()
	membership.add(testOrgID, "member-1")
	permChecker.grant(testOrgID, "member-1", "organization:manage")

	mount, err := svc.Execute(context.Background(), application.CreateSecretMountInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", Name: "primary", BackendType: "vault",
		Address: "http://vault:8200", RoleID: "role-1", SecretID: "s.super-secret-id",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mount.Name != "primary" || mount.BackendType != domain.BackendTypeVault {
		t.Errorf("unexpected mount fields: %+v", mount)
	}

	// The one real guarantee this whole context exists for: the
	// plaintext secret_id genuinely never reaches storage as-is.
	if bytes.Contains(mount.EncryptedSecretID, []byte("s.super-secret-id")) {
		t.Errorf("EncryptedSecretID must never contain the plaintext secret_id, got sealed bytes that do")
	}

	persisted, err := repo.GetByID(context.Background(), testOrgID, mount.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if bytes.Contains(persisted.EncryptedSecretID, []byte("s.super-secret-id")) {
		t.Errorf("persisted EncryptedSecretID must never contain the plaintext secret_id")
	}
}
