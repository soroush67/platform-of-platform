package http_test

import (
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/tenancy/adapters/http"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func TestCreateTeamHandler_ForbiddenWithoutPermission(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	svc := application.NewCreateTeamService(newFakeTeamRepo(), membershipRepo, newFakePermissionChecker())
	handler := withAuth(httpadapter.CreateTeamHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/teams", "user-1", []byte(`{"name":"Platform"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 403 {
		t.Fatalf("expected 403 without organization:manage, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateTeamHandler_Succeeds(t *testing.T) {
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewCreateTeamService(newFakeTeamRepo(), membershipRepo, permChecker)
	handler := withAuth(httpadapter.CreateTeamHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/teams", "user-1", []byte(`{"name":"Platform"}`))
	req.SetPathValue("id", "org-1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddTeamMemberHandler_UnknownTeamReturnsNotFound(t *testing.T) {
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	membershipRepo.add("org-1", "user-2")
	svc := application.NewAddTeamMemberService(newFakeTeamRepo(), membershipRepo, permChecker)
	handler := withAuth(httpadapter.AddTeamMemberHandler(svc))

	req := authedRequest(t, "POST", "/api/v1/orgs/org-1/teams/nonexistent/members", "user-1", []byte(`{"user_id":"user-2"}`))
	req.SetPathValue("id", "org-1")
	req.SetPathValue("team", "nonexistent")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for an unknown team, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveTeamMemberHandler_Succeeds(t *testing.T) {
	teamRepo := newFakeTeamRepo()
	team, _ := domain.NewTeam("org-1", "Platform")
	teamRepo.put(team)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add("org-1", "user-1")
	permChecker := newFakePermissionChecker()
	permChecker.grant("org-1", "user-1", "organization:manage")
	svc := application.NewRemoveTeamMemberService(teamRepo, membershipRepo, permChecker)
	handler := withAuth(httpadapter.RemoveTeamMemberHandler(svc))

	req := authedRequest(t, "DELETE", "/api/v1/orgs/org-1/teams/"+team.ID+"/members/user-2", "user-1", nil)
	req.SetPathValue("id", "org-1")
	req.SetPathValue("team", team.ID)
	req.SetPathValue("user_id", "user-2")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}
