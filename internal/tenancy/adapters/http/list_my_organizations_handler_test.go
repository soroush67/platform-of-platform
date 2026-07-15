package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/tenancy/adapters/http"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func TestListMyOrganizationsHandler_NoAuthGets401(t *testing.T) {
	svc := application.NewListMyOrganizationsService(newFakeRootMembershipRepo())
	handler := withAuth(httpadapter.ListMyOrganizationsHandler(svc))

	req := httptest.NewRequest("GET", "/api/v1/orgs", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401 with no Authorization header, got %d", rec.Code)
	}
}

func TestListMyOrganizationsHandler_Succeeds(t *testing.T) {
	repo := newFakeRootMembershipRepo()
	org, _ := domain.NewOrganization("Org A", "org-a")
	repo.addMembership("user-1", org)
	svc := application.NewListMyOrganizationsService(repo)
	handler := withAuth(httpadapter.ListMyOrganizationsHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs", "user-1", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0]["slug"] != "org-a" {
		t.Errorf("expected exactly the caller's own org in the response, got %+v", body.Data)
	}
}

func TestListMyOrganizationsHandler_EmptyMembershipReturnsEmptyListNotNull(t *testing.T) {
	svc := application.NewListMyOrganizationsService(newFakeRootMembershipRepo())
	handler := withAuth(httpadapter.ListMyOrganizationsHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs", "user-with-no-orgs", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data == nil {
		t.Error("expected an empty array, not a null data field")
	}
	if len(body.Data) != 0 {
		t.Errorf("expected zero orgs, got %d", len(body.Data))
	}
}
