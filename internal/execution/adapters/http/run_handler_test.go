package http_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/execution/adapters/http"
	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
)

func TestTriggerRunHandler_UnknownWorkspaceReturnsNotFound(t *testing.T) {
	svc := application.NewTriggerRunService(newFakeRunRepo(), newFakeWorkspaceLocker(), newFakeWorkspaceChecker(), newFakeScopedPermissionChecker(), newFakeOrganizationChecker())
	handler := withAuth(httpadapter.TriggerRunHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/runs", "user-1", []byte(`{}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", "ws-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for an unknown workspace, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerRunHandler_Succeeds(t *testing.T) {
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add("org-1", "project-1", "ws-1")
	permChecker := newFakeScopedPermissionChecker()
	permChecker.grant("org-1", "user-1", "workspace:apply")
	runRepo := newFakeRunRepo()
	svc := application.NewTriggerRunService(runRepo, newFakeWorkspaceLocker(), workspaceChecker, permChecker, newFakeOrganizationChecker())
	handler := withAuth(httpadapter.TriggerRunHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/runs", "user-1", []byte(`{}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", "ws-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "queued" {
		t.Errorf("expected a freshly triggered run to be queued, got %+v", body)
	}
}

func TestCancelRunHandler_RejectsRunFromAnotherWorkspace(t *testing.T) {
	runRepo := newFakeRunRepo()
	run, _ := domain.NewRun("org-1", "a-different-workspace", "user-1")
	runRepo.put(run)
	permChecker := newFakeScopedPermissionChecker()
	permChecker.grant("org-1", "user-1", "workspace:apply")
	svc := application.NewCancelRunService(runRepo, newFakeWorkspaceLocker(), permChecker, &fakeWorkerCanceler{})
	handler := withAuth(httpadapter.CancelRunHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/runs/"+run.ID+"/cancel", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", "ws-1")
	req.SetPathValue("runID", run.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a run under a different workspace, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelRunHandler_Succeeds(t *testing.T) {
	runRepo := newFakeRunRepo()
	run, _ := domain.NewRun("org-1", "ws-1", "user-1")
	runRepo.put(run)
	locker := newFakeWorkspaceLocker()
	_, _ = locker.TryLock(context.Background(), "org-1", "ws-1", run.ID)
	permChecker := newFakeScopedPermissionChecker()
	permChecker.grant("org-1", "user-1", "workspace:apply")
	svc := application.NewCancelRunService(runRepo, locker, permChecker, &fakeWorkerCanceler{})
	handler := withAuth(httpadapter.CancelRunHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/runs/"+run.ID+"/cancel", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", "ws-1")
	req.SetPathValue("runID", run.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "canceled" {
		t.Errorf("expected the run to be canceled, got %+v", body)
	}
}

func TestListRunsHandler_Succeeds(t *testing.T) {
	runRepo := newFakeRunRepo()
	run, _ := domain.NewRun("org-1", "ws-1", "user-1")
	runRepo.put(run)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add("org-1", "project-1", "ws-1")
	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant("org-1", "user-1", "project:read", "project", "project-1")
	svc := application.NewListRunsService(runRepo, membership, workspaceChecker, newFakePermissionChecker(), visibilityChecker)
	handler := withAuth(httpadapter.ListRunsHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/runs", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", "ws-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetRunHandler_NonMemberGetsNotFound(t *testing.T) {
	runRepo := newFakeRunRepo()
	run, _ := domain.NewRun("org-1", "ws-1", "user-1")
	runRepo.put(run)
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add("org-1", "project-1", "ws-1")
	svc := application.NewGetRunService(runRepo, newFakeMembershipChecker(), workspaceChecker, newFakePermissionChecker(), newFakeVisibilityChecker())
	handler := withAuth(httpadapter.GetRunHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/runs/"+run.ID, "stranger", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("projectID", "project-1")
	req.SetPathValue("workspaceID", "ws-1")
	req.SetPathValue("runID", run.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a non-member, got %d: %s", rec.Code, rec.Body.String())
	}
}
