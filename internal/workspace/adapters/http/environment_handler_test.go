package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/workspace/adapters/http"
	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

func TestCreateEnvironmentHandler_ForbiddenWithoutPermission(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	svc := application.NewCreateEnvironmentService(newFakeEnvironmentRepo(), membership, newFakePermissionChecker(), projectChecker)
	handler := withAuth(httpadapter.CreateEnvironmentHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/environments", "user-1", []byte(`{"name":"dev"}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEnvironmentHandler_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateEnvironmentService(newFakeEnvironmentRepo(), membership, permChecker, projectChecker)
	handler := withAuth(httpadapter.CreateEnvironmentHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/environments", "user-1", []byte(`{"name":"dev","promotion_rank":0}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["name"] != "dev" {
		t.Errorf("expected the created environment's name in the response, got %+v", body)
	}
}

func TestListEnvironmentsHandler_Succeeds(t *testing.T) {
	envRepo := newFakeEnvironmentRepo()
	env, _ := domain.NewEnvironment("org-1", "project-1", "dev", 0, false)
	envRepo.put(env)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	svc := application.NewListEnvironmentsService(envRepo, membership, projectChecker)
	handler := withAuth(httpadapter.ListEnvironmentsHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/environments", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetEnvironmentHandler_WrongProjectReturnsNotFound(t *testing.T) {
	envRepo := newFakeEnvironmentRepo()
	env, _ := domain.NewEnvironment("org-1", "a-different-project", "dev", 0, false)
	envRepo.put(env)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	svc := application.NewGetEnvironmentService(envRepo, membership, projectChecker)
	handler := withAuth(httpadapter.GetEnvironmentHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/environments/"+env.ID, "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("envID", env.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for an environment under a different project, got %d: %s", rec.Code, rec.Body.String())
	}
}
