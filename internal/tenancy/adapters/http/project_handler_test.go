package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/tenancy/adapters/http"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func TestCreateProjectHandler_ForbiddenWithoutPermission(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	svc := application.NewCreateProjectService(newFakeProjectRepo(), membershipRepo, newFakePermissionChecker(), newFakeOrgRepo())
	handler := withAuth(httpadapter.CreateProjectHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects", "user-1", []byte(`{"name":"P","slug":"p"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateProjectHandler_Succeeds(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add(org.ID, "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(org.ID, "user-1", "organization:manage")
	svc := application.NewCreateProjectService(newFakeProjectRepo(), membershipRepo, permChecker, orgRepo)
	handler := withAuth(httpadapter.CreateProjectHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/"+org.ID+"/projects", "user-1", []byte(`{"name":"P","slug":"p"}`))
	req.SetPathValue("id", org.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["name"] != "P" || body["slug"] != "p" {
		t.Errorf("expected the created project's fields in the response, got %+v", body)
	}
}

func TestListProjectsHandler_Succeeds(t *testing.T) {
	projectRepo := newFakeProjectRepo()
	p, _ := domain.NewProject("org-1", "P", "p", "")
	projectRepo.put(p)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	svc := application.NewListProjectsService(projectRepo, membershipRepo)
	handler := withAuth(httpadapter.ListProjectsHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects", "user-1", nil)
	req.SetPathValue("id", "org-1")
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
	if len(body.Data) != 1 {
		t.Fatalf("expected exactly 1 project, got %d", len(body.Data))
	}
}

func TestGetProjectHandler_UnknownReturnsNotFound(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	svc := application.NewGetProjectService(newFakeProjectRepo(), membershipRepo)
	handler := withAuth(httpadapter.GetProjectHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/nonexistent", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "nonexistent")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
