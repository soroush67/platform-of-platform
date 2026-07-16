package httpserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
)

var testSecret = []byte("auth-middleware-test-secret")

func noopHandler(userIDSeen *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id, ok := httpserver.UserIDFromContext(r.Context()); ok {
			*userIDSeen = id
		}
		w.WriteHeader(http.StatusOK)
	}
}

func TestRequireAuthOrFirstUserBootstrap_NoTokenAndZeroUsers_Allowed(t *testing.T) {
	var seen string
	countUsers := func(ctx context.Context) (int, error) { return 0, nil }
	handler := httpserver.RequireAuthOrFirstUserBootstrap(testSecret, nil, countUsers, noopHandler(&seen))

	req := httptest.NewRequest("POST", "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (bootstrap allowed), got %d: %s", rec.Code, rec.Body.String())
	}
	if seen != "" {
		t.Errorf("expected no subject id in context for an unauthenticated bootstrap request, got %q", seen)
	}
}

func TestRequireAuthOrFirstUserBootstrap_NoTokenAndUsersExist_Rejected(t *testing.T) {
	countUsers := func(ctx context.Context) (int, error) { return 1, nil }
	handler := httpserver.RequireAuthOrFirstUserBootstrap(testSecret, nil, countUsers, noopHandler(new(string)))

	req := httptest.NewRequest("POST", "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 once a user already exists, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAuthOrFirstUserBootstrap_ValidTokenAlwaysWorks_RegardlessOfUserCount(t *testing.T) {
	var seen string
	token, err := auth.IssueAccessToken(testSecret, "real-user-id")
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	// countUsers deliberately reports >0 - a valid token should never
	// even reach the bootstrap-eligibility check.
	countUsers := func(ctx context.Context) (int, error) { return 5, nil }
	handler := httpserver.RequireAuthOrFirstUserBootstrap(testSecret, nil, countUsers, noopHandler(&seen))

	req := httptest.NewRequest("POST", "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a valid token, got %d: %s", rec.Code, rec.Body.String())
	}
	if seen != "real-user-id" {
		t.Errorf("expected the real authenticated subject id in context, got %q", seen)
	}
}

func TestRequireAuthOrFirstUserBootstrap_InvalidTokenNeverFallsBackToBootstrap(t *testing.T) {
	// A PRESENT but invalid token must hard-fail, not be treated as "no
	// token" and silently fall through to the bootstrap check - even
	// with zero users, a garbled Authorization header is a real error,
	// not an invitation to bootstrap.
	countUsers := func(ctx context.Context) (int, error) {
		t.Fatal("countUsers must not be called when a token was presented, even an invalid one")
		return 0, nil
	}
	handler := httpserver.RequireAuthOrFirstUserBootstrap(testSecret, nil, countUsers, noopHandler(new(string)))

	req := httptest.NewRequest("POST", "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an invalid token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAuthOrFirstUserBootstrap_CountErrorMapsTo500(t *testing.T) {
	countUsers := func(ctx context.Context) (int, error) { return 0, errors.New("db unavailable") }
	handler := httpserver.RequireAuthOrFirstUserBootstrap(testSecret, nil, countUsers, noopHandler(new(string)))

	req := httptest.NewRequest("POST", "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the bootstrap-eligibility check itself fails, got %d: %s", rec.Code, rec.Body.String())
	}
}
