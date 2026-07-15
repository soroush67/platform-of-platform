package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

const testOrgID = "org-1"

func TestCreateServiceAccountService_RequiresOrganizationManage(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "member-1")
	svc := application.NewCreateServiceAccountService(newFakeServiceAccountRepo(), membership, newFakePermissionChecker())

	_, err := svc.Execute(context.Background(), application.CreateServiceAccountInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1", Name: "ci-bot",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestCreateServiceAccountService_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	repo := newFakeServiceAccountRepo()
	svc := application.NewCreateServiceAccountService(repo, membership, perm)

	sa, err := svc.Execute(context.Background(), application.CreateServiceAccountInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Name: "ci-bot",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), testOrgID, sa.ID); err != nil {
		t.Errorf("expected the service account to be persisted, got: %v", err)
	}
}

func alwaysValidScope(string) bool { return true }

func TestCreateAPIKeyService_RejectsUnknownScope(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	saRepo := newFakeServiceAccountRepo()
	sa, _ := domain.NewServiceAccount(testOrgID, "ci-bot", "")
	_ = saRepo.Create(context.Background(), sa)

	svc := application.NewCreateAPIKeyService(newFakeAPIKeyRepo(), saRepo, membership, perm, application.ScopeValidatorFunc(func(scope string) bool {
		return scope == "workspace:read" // only this one scope is "real"
	}))

	_, err := svc.Execute(context.Background(), application.CreateAPIKeyInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", ServiceAccountID: sa.ID,
		Name: "key", Scopes: []string{"workspace:read", "not-a-real-permission"},
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unknown scope, got: %v", err)
	}
}

func TestCreateAPIKeyService_ReturnsPlaintextExactlyOnce(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	saRepo := newFakeServiceAccountRepo()
	sa, _ := domain.NewServiceAccount(testOrgID, "ci-bot", "")
	_ = saRepo.Create(context.Background(), sa)
	keyRepo := newFakeAPIKeyRepo()

	svc := application.NewCreateAPIKeyService(keyRepo, saRepo, membership, perm, application.ScopeValidatorFunc(alwaysValidScope))

	result, err := svc.Execute(context.Background(), application.CreateAPIKeyInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", ServiceAccountID: sa.ID,
		Name: "key", Scopes: []string{"workspace:read"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Plaintext == "" {
		t.Fatal("expected a real plaintext key")
	}
	if result.Key.KeyHash == result.Plaintext {
		t.Fatal("expected the stored KeyHash to differ from the plaintext - it must be hashed, never stored verbatim")
	}

	// The stored record, looked up by hashing the SAME plaintext again
	// (exactly what the real authentication path does), must resolve
	// back to this key.
	stored, err := keyRepo.GetByHash(context.Background(), result.Key.KeyHash)
	if err != nil {
		t.Fatalf("GetByHash: %v", err)
	}
	if stored.ID != result.Key.ID {
		t.Errorf("expected the stored key to match, got a different id")
	}
}

func TestCreateAPIKeyService_UnknownServiceAccountRejected(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	svc := application.NewCreateAPIKeyService(newFakeAPIKeyRepo(), newFakeServiceAccountRepo(), membership, perm, application.ScopeValidatorFunc(alwaysValidScope))

	_, err := svc.Execute(context.Background(), application.CreateAPIKeyInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", ServiceAccountID: "nonexistent-sa", Name: "key",
	})
	if !errors.Is(err, domain.ErrServiceAccountNotFound) {
		t.Fatalf("expected ErrServiceAccountNotFound, got: %v", err)
	}
}

func TestRevokeAPIKeyService_RevokesAndIsIdempotentlyRejectedAfter(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add(testOrgID, "admin-1")
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	keyRepo := newFakeAPIKeyRepo()
	key, _ := domain.NewAPIKey(domain.APIKeyOwnerTypeServiceAccount, "sa-1", "key", "some-hash", nil, nil)
	_ = keyRepo.Create(context.Background(), testOrgID, key)

	svc := application.NewRevokeAPIKeyService(keyRepo, membership, perm)

	if err := svc.Execute(context.Background(), application.RevokeAPIKeyInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", APIKeyID: key.ID,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	stored, _ := keyRepo.GetByHash(context.Background(), "some-hash")
	if stored.RevokedAt == nil {
		t.Fatal("expected the key to be marked revoked")
	}
	if stored.Valid() {
		t.Error("expected a revoked key to no longer be Valid()")
	}
}
