package http_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

var testJWTSecret = []byte("http-adapter-test-secret")

func newReader(body []byte) io.Reader { return bytes.NewReader(body) }

func authedRequest(t *testing.T, method, path, userID string, body []byte) *http.Request {
	t.Helper()
	token, err := auth.IssueAccessToken(testJWTSecret, userID)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, newReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func withAuth(h http.HandlerFunc) http.HandlerFunc {
	return httpserver.RequireAuth(testJWTSecret, nil, h)
}

type fakeUserRepo struct {
	mu    sync.Mutex
	users map[string]*domain.User
}

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{users: map[string]*domain.User{}} }

func (f *fakeUserRepo) Create(ctx context.Context, u *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *u
	f.users[u.ID] = &cp
	return nil
}

func (f *fakeUserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Username == username {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUserRepo) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.PasswordHash = &passwordHash
	return nil
}

func (f *fakeUserRepo) put(u *domain.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *u
	f.users[u.ID] = &cp
}

func (f *fakeUserRepo) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return false, domain.ErrUserNotFound
	}
	return u.IsPlatformAdmin, nil
}

func (f *fakeUserRepo) SetPlatformAdmin(ctx context.Context, userID string, isAdmin bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.IsPlatformAdmin = isAdmin
	return nil
}

type fakeRefreshTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*domain.RefreshToken
}

func newFakeRefreshTokenRepo() *fakeRefreshTokenRepo {
	return &fakeRefreshTokenRepo{tokens: map[string]*domain.RefreshToken{}}
}

func (f *fakeRefreshTokenRepo) Create(ctx context.Context, t *domain.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.tokens[t.TokenHash] = &cp
	return nil
}

func (f *fakeRefreshTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[tokenHash]
	if !ok {
		return nil, domain.ErrRefreshTokenInvalid
	}
	cp := *t
	return &cp, nil
}

func (f *fakeRefreshTokenRepo) Revoke(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.ID == id {
			now := t.CreatedAt
			t.RevokedAt = &now
		}
	}
	return nil
}

type fakePasswordResetTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]*domain.PasswordResetToken
}

func newFakePasswordResetTokenRepo() *fakePasswordResetTokenRepo {
	return &fakePasswordResetTokenRepo{tokens: map[string]*domain.PasswordResetToken{}}
}

func (f *fakePasswordResetTokenRepo) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.tokens[t.TokenHash] = &cp
	return nil
}

func (f *fakePasswordResetTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[tokenHash]
	if !ok {
		return nil, domain.ErrPasswordResetTokenInvalid
	}
	cp := *t
	return &cp, nil
}

func (f *fakePasswordResetTokenRepo) MarkUsed(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.ID == id {
			now := t.CreatedAt
			t.UsedAt = &now
		}
	}
	return nil
}

type fakeServiceAccountRepo struct {
	mu  sync.Mutex
	sas map[string]*domain.ServiceAccount
}

func newFakeServiceAccountRepo() *fakeServiceAccountRepo {
	return &fakeServiceAccountRepo{sas: map[string]*domain.ServiceAccount{}}
}

func (f *fakeServiceAccountRepo) Create(ctx context.Context, sa *domain.ServiceAccount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *sa
	f.sas[sa.ID] = &cp
	return nil
}

func (f *fakeServiceAccountRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.ServiceAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sa, ok := f.sas[id]
	if !ok || sa.OrganizationID != organizationID {
		return nil, domain.ErrServiceAccountNotFound
	}
	cp := *sa
	return &cp, nil
}

func (f *fakeServiceAccountRepo) put(sa *domain.ServiceAccount) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *sa
	f.sas[sa.ID] = &cp
}

type fakeAPIKeyRepo struct {
	mu   sync.Mutex
	keys map[string]*domain.APIKey
	org  map[string]string
}

func newFakeAPIKeyRepo() *fakeAPIKeyRepo {
	return &fakeAPIKeyRepo{keys: map[string]*domain.APIKey{}, org: map[string]string{}}
}

func (f *fakeAPIKeyRepo) Create(ctx context.Context, organizationID string, key *domain.APIKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *key
	f.keys[key.ID] = &cp
	f.org[key.ID] = organizationID
	return nil
}

func (f *fakeAPIKeyRepo) GetByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range f.keys {
		if k.KeyHash == keyHash {
			cp := *k
			return &cp, nil
		}
	}
	return nil, domain.ErrAPIKeyInvalid
}

func (f *fakeAPIKeyRepo) TouchLastUsed(ctx context.Context, id string) error { return nil }

func (f *fakeAPIKeyRepo) Revoke(ctx context.Context, organizationID, keyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[keyID]
	if !ok || f.org[keyID] != organizationID || k.RevokedAt != nil {
		return domain.ErrAPIKeyInvalid
	}
	now := k.CreatedAt
	k.RevokedAt = &now
	return nil
}

type fakeMembershipChecker struct {
	mu      sync.Mutex
	members map[string]bool
}

func newFakeMembershipChecker() *fakeMembershipChecker {
	return &fakeMembershipChecker{members: map[string]bool{}}
}

func (f *fakeMembershipChecker) IsMember(ctx context.Context, organizationID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[organizationID+"|"+userID], nil
}

func (f *fakeMembershipChecker) add(orgID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[orgID+"|"+userID] = true
}

type fakePermissionChecker struct {
	mu    sync.Mutex
	perms map[string]bool
}

func newFakePermissionChecker() *fakePermissionChecker {
	return &fakePermissionChecker{perms: map[string]bool{}}
}

func (f *fakePermissionChecker) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perms[organizationID+"|"+userID+"|"+permission], nil
}

func (f *fakePermissionChecker) grant(orgID, userID, permission string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.perms[orgID+"|"+userID+"|"+permission] = true
}

func alwaysValidScope(string) bool { return true }
