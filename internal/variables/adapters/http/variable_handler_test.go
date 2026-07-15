package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/variables/adapters/http"
	"platform-of-platform/internal/variables/application"
	"platform-of-platform/internal/variables/domain"
)

func TestCreateVariableHandler_NonMemberGetsScopeNotFound(t *testing.T) {
	svc := application.NewCreateVariableService(newFakeVariableRepo(), newFakeMembershipChecker(), newFakeProjectChecker(), &fakeEnvironmentChecker{}, newFakeWorkspaceChecker(), newFakePermissionChecker(), &fakeOrganizationChecker{}, newFakeSecretMountChecker())
	handler := withAuth(httpadapter.CreateVariableHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/variables", "stranger", []byte(`{"scope_type":"organization","scope_id":"org-1","key":"FOO","category":"env_var","sensitivity":"plain","value":"bar"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a non-member, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateVariableHandler_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateVariableService(newFakeVariableRepo(), membership, newFakeProjectChecker(), &fakeEnvironmentChecker{}, newFakeWorkspaceChecker(), permChecker, &fakeOrganizationChecker{}, newFakeSecretMountChecker())
	handler := withAuth(httpadapter.CreateVariableHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/variables", "user-1", []byte(`{"scope_type":"organization","scope_id":"org-1","key":"FOO","category":"env_var","sensitivity":"plain","value":"bar"}`))
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
	if body["key"] != "FOO" || body["value"] != "bar" {
		t.Errorf("expected the created variable's fields in the response, got %+v", body)
	}
}

func TestCreateVariableHandler_SensitiveValueIsMasked(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateVariableService(newFakeVariableRepo(), membership, newFakeProjectChecker(), &fakeEnvironmentChecker{}, newFakeWorkspaceChecker(), permChecker, &fakeOrganizationChecker{}, newFakeSecretMountChecker())
	handler := withAuth(httpadapter.CreateVariableHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/variables", "user-1", []byte(`{"scope_type":"organization","scope_id":"org-1","key":"SECRET","category":"env_var","sensitivity":"sensitive","value":"topsecret"}`))
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
	if _, present := body["value"]; present && body["value"] != nil {
		t.Errorf("expected a sensitive variable's value to be masked (null), got %+v", body["value"])
	}
}

func TestUpdateVariableHandler_Succeeds(t *testing.T) {
	repo := newFakeVariableRepo()
	v, _ := domain.NewVariable("org-1", domain.ScopeTypeOrganization, "org-1", "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "original")
	repo.put(v)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewUpdateVariableService(repo, membership, permChecker)
	handler := withAuth(httpadapter.UpdateVariableHandler(svc))

	req := authedRequest(t, "PUT", "/api/v1/orgs/org-1/variables/"+v.ID, "user-1", []byte(`{"category":"env_var","sensitivity":"plain","value":"updated"}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("variableID", v.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["value"] != "updated" {
		t.Errorf("expected the updated value in the response, got %+v", body)
	}
}

func TestDeleteVariableHandler_Succeeds(t *testing.T) {
	repo := newFakeVariableRepo()
	v, _ := domain.NewVariable("org-1", domain.ScopeTypeOrganization, "org-1", "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "bar")
	repo.put(v)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewDeleteVariableService(repo, membership, permChecker)
	handler := withAuth(httpadapter.DeleteVariableHandler(svc))

	req := authedRequest(t, "DELETE", "/api/v1/orgs/org-1/variables/"+v.ID, "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("variableID", v.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListVariablesHandler_Succeeds(t *testing.T) {
	repo := newFakeVariableRepo()
	v, _ := domain.NewVariable("org-1", domain.ScopeTypeOrganization, "org-1", "FOO", domain.CategoryEnvVar, domain.SensitivityPlain, "bar")
	repo.put(v)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	svc := application.NewListVariablesService(repo, membership)
	handler := withAuth(httpadapter.ListVariablesHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/variables?scope_type=organization&scope_id=org-1", "user-1", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResolveVariableHandler_NotFoundInCascade(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	workspaceChecker := newFakeWorkspaceChecker()
	workspaceChecker.add("org-1", "ws-1")
	svc := application.NewResolveVariableService(newFakeVariableRepo(), membership, workspaceChecker, newFakeSecretResolver())
	handler := withAuth(httpadapter.ResolveVariableHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/projects/project-1/workspaces/ws-1/variables/resolve?key=NOPE", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("workspaceID", "ws-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 when no variable resolves anywhere in the cascade, got %d: %s", rec.Code, rec.Body.String())
	}
}
