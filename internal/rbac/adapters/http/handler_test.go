package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/rbac/adapters/http"
	"platform-of-platform/internal/rbac/application"
	"platform-of-platform/internal/rbac/domain"
)

func TestCreateRoleHandler_RejectsUnknownPermission(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateRoleService(newFakeRoleRepo(), membership, permChecker)
	handler := withAuth(httpadapter.CreateRoleHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/roles", "user-1", []byte(`{"name":"custom","permissions":["not:a:real:permission"]}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an unknown permission, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRoleHandler_Succeeds(t *testing.T) {
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateRoleService(newFakeRoleRepo(), membership, permChecker)
	handler := withAuth(httpadapter.CreateRoleHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/roles", "user-1", []byte(`{"name":"deployer","permissions":["workspace:apply"]}`))
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
	if body["name"] != "deployer" {
		t.Errorf("expected the created role's name in the response, got %+v", body)
	}
}

func TestListRolesHandler_RequiresMembership(t *testing.T) {
	svc := application.NewListRolesService(newFakeRoleRepo(), newFakeMembershipChecker())
	handler := withAuth(httpadapter.ListRolesHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/roles", "stranger", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 for a non-member, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListRolesHandler_Succeeds(t *testing.T) {
	roleRepo := newFakeRoleRepo()
	role, _ := domain.NewRole("org-1", "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(role)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	svc := application.NewListRolesService(roleRepo, membership)
	handler := withAuth(httpadapter.ListRolesHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/roles", "user-1", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRoleBindingHandler_DefaultsToAllowEffect(t *testing.T) {
	roleRepo := newFakeRoleRepo()
	role, _ := domain.NewRole("org-1", "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(role)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	membership.add("org-1", "target-user")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	resourceChecker := newFakeResourceChecker()
	svc := application.NewCreateRoleBindingService(roleRepo, newFakeRoleBindingRepo(), membership, permChecker, resourceChecker, resourceChecker, resourceChecker, resourceChecker)
	handler := withAuth(httpadapter.CreateRoleBindingHandler(svc))

	reqBody := []byte(`{"role_id":"` + role.ID + `","subject":{"type":"user","id":"target-user"},"scope":{"type":"organization","id":"org-1"}}`)
	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/role-bindings", "user-1", reqBody)
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
	if body["effect"] != "allow" {
		t.Errorf("expected the effect to default to allow, got %+v", body)
	}
}

func TestCreateRoleBindingHandler_RoleFromAnotherOrgReturnsNotFound(t *testing.T) {
	roleRepo := newFakeRoleRepo()
	foreignRole, _ := domain.NewRole("org-2", "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(foreignRole)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	membership.add("org-1", "target-user")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	resourceChecker := newFakeResourceChecker()
	svc := application.NewCreateRoleBindingService(roleRepo, newFakeRoleBindingRepo(), membership, permChecker, resourceChecker, resourceChecker, resourceChecker, resourceChecker)
	handler := withAuth(httpadapter.CreateRoleBindingHandler(svc))

	reqBody := []byte(`{"role_id":"` + foreignRole.ID + `","subject":{"type":"user","id":"target-user"},"scope":{"type":"organization","id":"org-1"}}`)
	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/role-bindings", "user-1", reqBody)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a role belonging to a different org, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListRoleBindingsHandler_Succeeds(t *testing.T) {
	bindingRepo := newFakeRoleBindingRepo()
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	roleRepo := newFakeRoleRepo()
	nameReader := newFakeNameReader()
	svc := application.NewListRoleBindingsService(bindingRepo, membership, roleRepo, newFakeUserReader(), nameReader, nameReader, nameReader, nameReader)
	handler := withAuth(httpadapter.ListRoleBindingsHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/role-bindings", "user-1", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteRoleBindingHandler_Succeeds(t *testing.T) {
	bindingRepo := newFakeRoleBindingRepo()
	binding := domain.NewRoleBinding("org-1", "role-1", domain.SubjectTypeUser, "target-user", domain.ScopeTypeOrganization, "org-1", domain.EffectAllow)
	bindingRepo.put(binding)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewDeleteRoleBindingService(bindingRepo, membership, permChecker)
	handler := withAuth(httpadapter.DeleteRoleBindingHandler(svc))

	req := authedRequest(t, "DELETE", "/api/v1/orgs/org-1/role-bindings/"+binding.ID, "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("bindingID", binding.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRoleHandler_RejectsBuiltinRole(t *testing.T) {
	roleRepo := newFakeRoleRepo()
	builtin, _ := domain.NewRole("org-1", "owner", []domain.Permission{domain.PermissionOrganizationManage})
	builtin.OrganizationID = nil
	roleRepo.put(builtin)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewUpdateRoleService(roleRepo, membership, permChecker)
	handler := withAuth(httpadapter.UpdateRoleHandler(svc))

	req := authedRequest(t, "PUT", "/api/v1/orgs/org-1/roles/"+builtin.ID, "user-1", []byte(`{"permissions":["workspace:read"]}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("role", builtin.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 for a built-in role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRoleHandler_Succeeds(t *testing.T) {
	roleRepo := newFakeRoleRepo()
	role, _ := domain.NewRole("org-1", "deployer", []domain.Permission{domain.PermissionWorkspaceApply})
	roleRepo.put(role)
	membership := newFakeMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewUpdateRoleService(roleRepo, membership, permChecker)
	handler := withAuth(httpadapter.UpdateRoleHandler(svc))

	req := authedRequest(t, "PUT", "/api/v1/orgs/org-1/roles/"+role.ID, "user-1", []byte(`{"permissions":["workspace:read","workspace:manage"]}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("role", role.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["permissions"] == nil {
		t.Errorf("expected updated permissions in the response, got %+v", body)
	}
}
