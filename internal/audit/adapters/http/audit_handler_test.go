package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/audit/adapters/http"
	"platform-of-platform/internal/audit/application"
	"platform-of-platform/internal/audit/domain"
)

func TestListAuditLogHandler_ForbiddenWithoutPermission(t *testing.T) {
	svc := application.NewListAuditEntriesService(newFakeAuditEntryRepo(), newFakePermissionChecker())
	handler := withAuth(httpadapter.ListAuditLogHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/audit-log", "user-1", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListAuditLogHandler_InvalidLimitMapsTo400(t *testing.T) {
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewListAuditEntriesService(newFakeAuditEntryRepo(), permChecker)
	handler := withAuth(httpadapter.ListAuditLogHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/audit-log?limit=-5", "user-1", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for a negative limit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListAuditLogHandler_Succeeds(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	entry := domain.NewEntry("org-1", "event-1", "user-1", "OrganizationCreated", "organization", "org-1", map[string]any{})
	repo.put(entry)
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewListAuditEntriesService(repo, permChecker)
	handler := withAuth(httpadapter.ListAuditLogHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/audit-log", "user-1", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected exactly 1 audit entry, got %d", len(data))
	}
}
