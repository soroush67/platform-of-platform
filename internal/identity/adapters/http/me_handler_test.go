package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	httpadapter "platform-of-platform/internal/identity/adapters/http"
	"platform-of-platform/internal/identity/application"
)

func TestGetOwnUserHandler_NoAuthGets401(t *testing.T) {
	svc := application.NewGetOwnUserService(newFakeUserRepo())
	handler := withAuth(httpadapter.GetOwnUserHandler(svc))

	req := httptest.NewRequest("GET", "/api/v1/users/me", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401 with no Authorization header, got %d", rec.Code)
	}
}

func TestGetOwnUserHandler_Succeeds(t *testing.T) {
	repo := newFakeUserRepo()
	user := mustLocalUser(t, "alice", "hunter2")
	repo.put(user)
	svc := application.NewGetOwnUserService(repo)
	handler := withAuth(httpadapter.GetOwnUserHandler(svc))

	req := authedRequest(t, "GET", "/api/v1/users/me", user.ID, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["username"] != "alice" {
		t.Errorf("expected the caller's own username in the response, got %+v", body)
	}
	if _, present := body["password_hash"]; present {
		t.Errorf("expected password_hash to never be serialized, got %+v", body)
	}
}

func TestGetOwnUserHandler_UnknownUserGets404(t *testing.T) {
	svc := application.NewGetOwnUserService(newFakeUserRepo())
	handler := withAuth(httpadapter.GetOwnUserHandler(svc))

	// A syntactically valid JWT for a user id that doesn't exist in the
	// repo - the defensive "deleted account" edge case GetOwnUserService
	// itself doesn't special-case, just propagates.
	req := authedRequest(t, "GET", "/api/v1/users/me", "deleted-user-id", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404 for an unknown user id, got %d: %s", rec.Code, rec.Body.String())
	}
}
