package http_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/identity/adapters/http"
	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

func TestCreateServiceAccountHandler_ForbiddenWithoutPermission(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	svc := application.NewCreateServiceAccountService(newFakeServiceAccountRepo(), membership, newFakePermissionChecker())
	handler := withAuth(httpadapter.CreateServiceAccountHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/service-accounts", "user-1", []byte(`{"name":"ci-bot"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateServiceAccountHandler_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateServiceAccountService(newFakeServiceAccountRepo(), membership, permChecker)
	handler := withAuth(httpadapter.CreateServiceAccountHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/service-accounts", "user-1", []byte(`{"name":"ci-bot"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["name"] != "ci-bot" {
		t.Errorf("expected the created service account's name in the response, got %+v", body)
	}
}

func TestCreateAPIKeyHandler_ReturnsPlaintextExactlyOnce(t *testing.T) {
	saRepo := newFakeServiceAccountRepo()
	sa, _ := domain.NewServiceAccount("org-1", "ci-bot", "")
	saRepo.put(sa)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateAPIKeyService(newFakeAPIKeyRepo(), saRepo, membership, permChecker, application.ScopeValidatorFunc(alwaysValidScope))
	handler := withAuth(httpadapter.CreateAPIKeyHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/service-accounts/"+sa.ID+"/api-keys", "user-1", []byte(`{"name":"ci-key","scopes":["workspace:read"]}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("sa", sa.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	key, _ := body["key"].(string)
	if key == "" {
		t.Error("expected the create response to carry the plaintext key")
	}
}

func TestCreateAPIKeyHandler_UnknownServiceAccountReturnsNotFound(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateAPIKeyService(newFakeAPIKeyRepo(), newFakeServiceAccountRepo(), membership, permChecker, application.ScopeValidatorFunc(alwaysValidScope))
	handler := withAuth(httpadapter.CreateAPIKeyHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/service-accounts/nonexistent/api-keys", "user-1", []byte(`{"name":"ci-key"}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("sa", "nonexistent")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for an unknown service account, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeAPIKeyHandler_Succeeds(t *testing.T) {
	apiKeyRepo := newFakeAPIKeyRepo()
	key, _ := domain.NewAPIKey(domain.APIKeyOwnerTypeServiceAccount, "sa-1", "ci-key", "a-hash", nil, nil)
	_ = apiKeyRepo.Create(context.Background(), "org-1", key)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewRevokeAPIKeyService(apiKeyRepo, membership, permChecker)
	handler := withAuth(httpadapter.RevokeAPIKeyHandler(svc))

	req := authedRequest(t, "DELETE", "/api/v1/orgs/org-1/service-accounts/sa-1/api-keys/"+key.ID, "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("sa", "sa-1")
	req.SetPathValue("key", key.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}
