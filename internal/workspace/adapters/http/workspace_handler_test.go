package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/workspace/adapters/http"
	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

func TestCreateWorkspaceHandler_ForbiddenWithoutPermission(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	svc := application.NewCreateWorkspaceService(newFakeWorkspaceRepo(), newFakeEnvironmentRepo(), membership, newFakePermissionChecker(), projectChecker, newFakeOrganizationChecker())
	handler := withAuth(httpadapter.CreateWorkspaceHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/workspaces", "user-1", []byte(`{"name":"ws","execution_engine":"compose"}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without workspace:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateWorkspaceHandler_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "workspace:manage")
	svc := application.NewCreateWorkspaceService(newFakeWorkspaceRepo(), newFakeEnvironmentRepo(), membership, permChecker, projectChecker, newFakeOrganizationChecker())
	handler := withAuth(httpadapter.CreateWorkspaceHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/workspaces", "user-1", []byte(`{"name":"ws","execution_engine":"compose"}`))
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
	if body["name"] != "ws" || body["execution_engine"] != "compose" {
		t.Errorf("expected the created workspace's fields in the response, got %+v", body)
	}
}

func TestListWorkspacesHandler_Succeeds(t *testing.T) {
	wsRepo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace("org-1", "project-1", nil, "ws", domain.ExecutionEngineCompose)
	wsRepo.put(ws)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	svc := application.NewListWorkspacesService(wsRepo, membership, projectChecker)
	handler := withAuth(httpadapter.ListWorkspacesHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/workspaces", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetWorkspaceHandler_WrongProjectReturnsNotFound(t *testing.T) {
	wsRepo := newFakeWorkspaceRepo()
	ws, _ := domain.NewWorkspace("org-1", "a-different-project", nil, "ws", domain.ExecutionEngineCompose)
	wsRepo.put(ws)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	projectChecker := newFakeProjectChecker()
	projectChecker.add("org-1", "project-1")
	svc := application.NewGetWorkspaceService(wsRepo, membership, projectChecker)
	handler := withAuth(httpadapter.GetWorkspaceHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/workspaces/"+ws.ID, "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", ws.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a workspace under a different project, got %d: %s", rec.Code, rec.Body.String())
	}
}
