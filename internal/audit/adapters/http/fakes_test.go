package http_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"platform-of-platform/internal/audit/domain"
	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
)

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

type fakeAuditEntryRepo struct {
	mu      sync.Mutex
	entries []*domain.Entry
}

func newFakeAuditEntryRepo() *fakeAuditEntryRepo { return &fakeAuditEntryRepo{} }

func (f *fakeAuditEntryRepo) Create(ctx context.Context, entry *domain.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *entry
	f.entries = append(f.entries, &cp)
	return nil
}

func (f *fakeAuditEntryRepo) ListByOrganization(ctx context.Context, organizationID string, limit int, beforeCreatedAt *time.Time, beforeID *string) ([]*domain.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Entry
	for _, e := range f.entries {
		if e.OrganizationID == organizationID {
			cp := *e
			out = append(out, &cp)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeAuditEntryRepo) put(e *domain.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	f.entries = append(f.entries, &cp)
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
