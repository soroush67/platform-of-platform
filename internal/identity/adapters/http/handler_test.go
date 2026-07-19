package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	httpadapter "platform-of-platform/internal/identity/adapters/http"
	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/ratelimit"
)

func mustLocalUser(t *testing.T, username, password string) *domain.User {
	t.Helper()
	u, err := domain.NewUser(username, username+"@example.com", domain.AuthSourceLocal)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	if err := u.SetPasswordHash(string(hash)); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	return u
}

func TestCreateUserHandler_InvalidJSONBody(t *testing.T) {
	svc := application.NewCreateUserService(newFakeUserRepo(), &fakeDefaultOrgBootstrapper{})
	handler := httpadapter.CreateUserHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/users", newReader([]byte(`not json`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateUserHandler_ValidationErrorMapsTo400(t *testing.T) {
	svc := application.NewCreateUserService(newFakeUserRepo(), &fakeDefaultOrgBootstrapper{})
	handler := httpadapter.CreateUserHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/users", newReader([]byte(`{"username":"bob","email":"not-an-email","auth_source":"local","password":"hunter22"}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 400 {
		t.Fatalf("expected 400 for an invalid email, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateUserHandler_Succeeds(t *testing.T) {
	svc := application.NewCreateUserService(newFakeUserRepo(), &fakeDefaultOrgBootstrapper{})
	handler := httpadapter.CreateUserHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/users", newReader([]byte(`{"username":"bob","email":"bob@example.com","auth_source":"local","password":"hunter22"}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["username"] != "bob" || body["email"] != "bob@example.com" {
		t.Errorf("expected the created user's fields in the response, got %+v", body)
	}
}

func TestLoginHandler_InvalidCredentialsMapsTo401(t *testing.T) {
	userRepo := newFakeUserRepo()
	authSvc := application.NewAuthenticateService(userRepo)
	refreshSvc := application.NewRefreshTokenService(newFakeRefreshTokenRepo(), userRepo)
	handler := httpadapter.LoginHandler(authSvc, refreshSvc, ratelimit.New(100, time.Minute), testJWTSecret)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", newReader([]byte(`{"username":"no-such-user","password":"x"}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginHandler_Succeeds(t *testing.T) {
	userRepo := newFakeUserRepo()
	u := mustLocalUser(t, "bob", "hunter22")
	userRepo.put(u)
	authSvc := application.NewAuthenticateService(userRepo)
	refreshSvc := application.NewRefreshTokenService(newFakeRefreshTokenRepo(), userRepo)
	handler := httpadapter.LoginHandler(authSvc, refreshSvc, ratelimit.New(100, time.Minute), testJWTSecret)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", newReader([]byte(`{"username":"bob","password":"hunter22"}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["access_token"] == "" || body["refresh_token"] == "" {
		t.Errorf("expected real access/refresh tokens in the response, got %+v", body)
	}
}

func TestLoginHandler_RateLimited(t *testing.T) {
	userRepo := newFakeUserRepo()
	authSvc := application.NewAuthenticateService(userRepo)
	refreshSvc := application.NewRefreshTokenService(newFakeRefreshTokenRepo(), userRepo)
	limiter := ratelimit.New(1, time.Minute)
	handler := httpadapter.LoginHandler(authSvc, refreshSvc, limiter, testJWTSecret)

	body := []byte(`{"username":"bob","password":"wrong"}`)
	handler(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/v1/auth/login", newReader(body)))

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest("POST", "/api/v1/auth/login", newReader(body)))
	if rec.Code != 429 {
		t.Fatalf("expected 429 on the second attempt against a 1-attempt limiter, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on a 429")
	}
}

func TestRequestPasswordResetHandler_AlwaysAccepted(t *testing.T) {
	svc := application.NewPasswordResetService(newFakePasswordResetTokenRepo(), newFakeUserRepo(), discardLogger())
	handler := httpadapter.RequestPasswordResetHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/auth/password-reset/request", newReader([]byte(`{"email":"no-such-user@example.com"}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 202 {
		t.Fatalf("expected 202 regardless of whether the email exists, got %d", rec.Code)
	}
}

func TestConfirmPasswordResetHandler_InvalidTokenMapsTo401(t *testing.T) {
	svc := application.NewPasswordResetService(newFakePasswordResetTokenRepo(), newFakeUserRepo(), discardLogger())
	handler := httpadapter.ConfirmPasswordResetHandler(svc)

	req := httptest.NewRequest("POST", "/api/v1/auth/password-reset/confirm", newReader([]byte(`{"token":"no-such-token","new_password":"a-real-new-password"}`)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 401 {
		t.Fatalf("expected 401 for an unknown reset token, got %d: %s", rec.Code, rec.Body.String())
	}
}
