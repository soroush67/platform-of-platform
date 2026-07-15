package http_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/variables/domain"
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

type fakeVariableRepo struct {
	mu        sync.Mutex
	variables map[string]*domain.Variable
}

func newFakeVariableRepo() *fakeVariableRepo {
	return &fakeVariableRepo{variables: map[string]*domain.Variable{}}
}

func (f *fakeVariableRepo) Create(ctx context.Context, v *domain.Variable) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *v
	f.variables[v.ID] = &cp
	return nil
}

func (f *fakeVariableRepo) GetByScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID, key string) (*domain.Variable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.variables {
		if v.OrganizationID == organizationID && v.ScopeType == scopeType && v.ScopeID == scopeID && v.Key == key {
			cp := *v
			return &cp, nil
		}
	}
	return nil, domain.ErrVariableNotFound
}

func (f *fakeVariableRepo) ListByScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID string) ([]*domain.Variable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Variable
	for _, v := range f.variables {
		if v.OrganizationID == organizationID && v.ScopeType == scopeType && v.ScopeID == scopeID {
			cp := *v
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeVariableRepo) GetByID(ctx context.Context, organizationID, variableID string) (*domain.Variable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.variables[variableID]
	if !ok || v.OrganizationID != organizationID {
		return nil, domain.ErrVariableNotFound
	}
	cp := *v
	return &cp, nil
}

func (f *fakeVariableRepo) Update(ctx context.Context, v *domain.Variable) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.variables[v.ID]; !ok {
		return domain.ErrVariableNotFound
	}
	cp := *v
	f.variables[v.ID] = &cp
	return nil
}

func (f *fakeVariableRepo) Delete(ctx context.Context, organizationID, variableID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.variables[variableID]
	if !ok || v.OrganizationID != organizationID {
		return domain.ErrVariableNotFound
	}
	delete(f.variables, variableID)
	return nil
}

func (f *fakeVariableRepo) put(v *domain.Variable) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *v
	f.variables[v.ID] = &cp
}

type fakeProjectChecker struct {
	mu       sync.Mutex
	projects map[string]bool
}

func newFakeProjectChecker() *fakeProjectChecker {
	return &fakeProjectChecker{projects: map[string]bool{}}
}

func (f *fakeProjectChecker) ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.projects[organizationID+"|"+projectID], nil
}

type fakeEnvironmentChecker struct{}

func (f *fakeEnvironmentChecker) Exists(ctx context.Context, organizationID, environmentID string) (bool, error) {
	return false, nil
}

type fakeWorkspaceChecker struct {
	mu     sync.Mutex
	scopes map[string]bool
}

func newFakeWorkspaceChecker() *fakeWorkspaceChecker {
	return &fakeWorkspaceChecker{scopes: map[string]bool{}}
}

func (f *fakeWorkspaceChecker) Exists(ctx context.Context, organizationID, workspaceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.scopes[organizationID+"|"+workspaceID], nil
}

func (f *fakeWorkspaceChecker) GetScope(ctx context.Context, organizationID, workspaceID string) (string, *string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.scopes[organizationID+"|"+workspaceID] {
		return "", nil, domain.ErrScopeNotFound
	}
	return "project-1", nil, nil
}

func (f *fakeWorkspaceChecker) add(orgID, workspaceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes[orgID+"|"+workspaceID] = true
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

type fakeOrganizationChecker struct{}

func (f *fakeOrganizationChecker) IsArchived(ctx context.Context, organizationID string) (bool, error) {
	return false, nil
}
