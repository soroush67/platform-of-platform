package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/tenancy/adapters/http"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func TestCreateOrganizationHandler_RequiresAuth(t *testing.T) {
	svc := application.NewCreateOrganizationService(newFakeOrgRepo(), newFakeMembershipRepo(), &fakeRoleAssigner{})
	handler := httpadapter.CreateOrganizationHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/orgs", newReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401 without an authenticated context, got %d", rec.Code)
	}
}

func TestCreateOrganizationHandler_InvalidJSONBody(t *testing.T) {
	svc := application.NewCreateOrganizationService(newFakeOrgRepo(), newFakeMembershipRepo(), &fakeRoleAssigner{})
	handler := withAuth(httpadapter.CreateOrganizationHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs", "user-1", []byte(`not json`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for malformed JSON, got %d", rec.Code)
	}
}

func TestCreateOrganizationHandler_ValidationErrorMapsTo400(t *testing.T) {
	svc := application.NewCreateOrganizationService(newFakeOrgRepo(), newFakeMembershipRepo(), &fakeRoleAssigner{})
	handler := withAuth(httpadapter.CreateOrganizationHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs", "user-1", []byte(`{"name":"Acme","slug":"NOT VALID"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an invalid slug, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateOrganizationHandler_Succeeds(t *testing.T) {
	svc := application.NewCreateOrganizationService(newFakeOrgRepo(), newFakeMembershipRepo(), &fakeRoleAssigner{})
	handler := withAuth(httpadapter.CreateOrganizationHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs", "user-1", []byte(`{"name":"Acme","slug":"acme"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["name"] != "Acme" || body["slug"] != "acme" || body["status"] != "active" {
		t.Errorf("expected the created org's fields in the response, got %+v", body)
	}
}

func TestGetOrganizationHandler_NonMemberGetsNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	svc := application.NewGetOrganizationService(orgRepo, newFakeMembershipRepo())
	handler := withAuth(httpadapter.GetOrganizationHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/"+org.ID, "stranger", nil)
	req.SetPathValue("id", org.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a non-member, got %d", rec.Code)
	}
}

func TestGetOrganizationHandler_Succeeds(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add(org.ID, "user-1")
	svc := application.NewGetOrganizationService(orgRepo, membershipRepo)
	handler := withAuth(httpadapter.GetOrganizationHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/"+org.ID, "user-1", nil)
	req.SetPathValue("id", org.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddMemberHandler_ForbiddenWithoutPermission(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	svc := application.NewAddMemberService(membershipRepo, newFakePermissionChecker(), &fakeRoleAssigner{})
	handler := withAuth(httpadapter.AddMemberHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/members", "user-1", []byte(`{"user_id":"user-2"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddMemberHandler_Succeeds(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewAddMemberService(membershipRepo, permChecker, &fakeRoleAssigner{})
	handler := withAuth(httpadapter.AddMemberHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/members", "user-1", []byte(`{"user_id":"user-2"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangeMemberRoleHandler_InvalidRoleNameMapsTo400(t *testing.T) {
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-2")
	svc := application.NewChangeMemberRoleService(membershipRepo, &fakeRoleChanger{}, permChecker)
	handler := withAuth(httpadapter.ChangeMemberRoleHandler(svc))

	req := authedRequest(t, "PUT", "/api/v1/orgs/org-1/members/user-2/role", "user-1", []byte(`{"role":"not-a-real-role"}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("userID", "user-2")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an unknown role name, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListMembersHandler_Succeeds(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	userReader := newFakeUserReader()
	userReader.set("user-1", "alice", "alice@example.com")
	roleReader := newFakeRoleReader()
	roleReader.set("org-1", "user-1", "owner")
	svc := application.NewListMembersService(membershipRepo, userReader, roleReader)
	handler := withAuth(httpadapter.ListMembersHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/members", "user-1", nil)
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
	if len(body.Data) != 1 || body.Data[0]["username"] != "alice" || body.Data[0]["role_name"] != "owner" {
		t.Fatalf("expected exactly one roster row for alice/owner, got %+v", body.Data)
	}
}

func TestListMembersHandler_NonMemberGetsNotFound(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	svc := application.NewListMembersService(membershipRepo, newFakeUserReader(), newFakeRoleReader())
	handler := withAuth(httpadapter.ListMembersHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/orgs/org-1/members", "not-a-member", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for a non-member requester, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListMembersHandler_RequiresAuth(t *testing.T) {
	svc := application.NewListMembersService(newFakeMembershipRepo(), newFakeUserReader(), newFakeRoleReader())
	handler := httpadapter.ListMembersHandler(svc)

	req := httptest.NewRequest("GET", "/api/v1/orgs/org-1/members", nil)
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401 with no auth context, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestArchiveOrganizationHandler_ForbiddenWithoutPermission(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add(org.ID, "user-1")
	svc := application.NewArchiveOrganizationService(orgRepo, orgRepo, membershipRepo, newFakePermissionChecker())
	handler := withAuth(httpadapter.ArchiveOrganizationHandler(svc))

	req := authedRequest(t, "DELETE", "/api/v1/orgs/"+org.ID, "user-1", nil)
	req.SetPathValue("id", org.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:delete, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestArchiveOrganizationHandler_Succeeds(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add(org.ID, "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant(org.ID, "user-1", "organization:delete")
	svc := application.NewArchiveOrganizationService(orgRepo, orgRepo, membershipRepo, permChecker)
	handler := withAuth(httpadapter.ArchiveOrganizationHandler(svc))

	req := authedRequest(t, "DELETE", "/api/v1/orgs/"+org.ID, "user-1", nil)
	req.SetPathValue("id", org.ID)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// A second archive of the same org must map ErrOrganizationAlreadyArchived to 409.
	req2 := authedRequest(t, "DELETE", "/api/v1/orgs/"+org.ID, "user-1", nil)
	req2.SetPathValue("id", org.ID)
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)
	if rec2.Code != 409 {
		t.Fatalf("expected 409 for an already-archived org, got %d: %s", rec2.Code, rec2.Body.String())
	}
}
