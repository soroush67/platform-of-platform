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
	"platform-of-platform/internal/workspace/domain"
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

type fakeEnvironmentRepo struct {
	mu   sync.Mutex
	envs map[string]*domain.Environment
}

func newFakeEnvironmentRepo() *fakeEnvironmentRepo {
	return &fakeEnvironmentRepo{envs: map[string]*domain.Environment{}}
}

func (f *fakeEnvironmentRepo) Create(ctx context.Context, env *domain.Environment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *env
	f.envs[env.ID] = &cp
	return nil
}

func (f *fakeEnvironmentRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Environment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.envs[id]
	if !ok || e.OrganizationID != organizationID {
		return nil, domain.ErrEnvironmentNotFound
	}
	cp := *e
	return &cp, nil
}

func (f *fakeEnvironmentRepo) ListByProject(ctx context.Context, organizationID, projectID string) ([]*domain.Environment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Environment
	for _, e := range f.envs {
		if e.OrganizationID == organizationID && e.ProjectID == projectID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeEnvironmentRepo) put(e *domain.Environment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	f.envs[e.ID] = &cp
}

type fakeWorkspaceRepo struct {
	mu         sync.Mutex
	workspaces map[string]*domain.Workspace
}

func newFakeWorkspaceRepo() *fakeWorkspaceRepo {
	return &fakeWorkspaceRepo{workspaces: map[string]*domain.Workspace{}}
}

func (f *fakeWorkspaceRepo) Create(ctx context.Context, ws *domain.Workspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *ws
	f.workspaces[ws.ID] = &cp
	return nil
}

func (f *fakeWorkspaceRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ws, ok := f.workspaces[id]
	if !ok || ws.OrganizationID != organizationID {
		return nil, domain.ErrWorkspaceNotFound
	}
	cp := *ws
	return &cp, nil
}

func (f *fakeWorkspaceRepo) ListByProject(ctx context.Context, organizationID, projectID string) ([]*domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Workspace
	for _, ws := range f.workspaces {
		if ws.OrganizationID == organizationID && ws.ProjectID == projectID {
			cp := *ws
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeWorkspaceRepo) put(ws *domain.Workspace) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *ws
	f.workspaces[ws.ID] = &cp
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

func (f *fakePermissionChecker) HasPermissionAtScope(ctx context.Context, organizationID, userID, permission string, projectID, workspaceID *string) (bool, error) {
	return f.HasPermission(ctx, organizationID, userID, permission)
}

func (f *fakePermissionChecker) grant(orgID, userID, permission string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.perms[orgID+"|"+userID+"|"+permission] = true
}

// fakeVisibilityChecker backs the new VisibilityChecker port
// (project_visibility.go) - same shape as the application-layer tests'
// own copy (a different package, can't share the type directly).
type fakeVisibilityChecker struct {
	mu     sync.Mutex
	grants map[string]bool
}

func newFakeVisibilityChecker() *fakeVisibilityChecker {
	return &fakeVisibilityChecker{grants: map[string]bool{}}
}

func (f *fakeVisibilityChecker) HasScopedPermission(ctx context.Context, organizationID, userID, permission, scopeType, scopeID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.grants[organizationID+"|"+userID+"|"+permission+"|"+scopeType+"|"+scopeID], nil
}

func (f *fakeVisibilityChecker) grant(orgID, userID, permission, scopeType, scopeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grants[orgID+"|"+userID+"|"+permission+"|"+scopeType+"|"+scopeID] = true
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

func (f *fakeProjectChecker) add(orgID, projectID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.projects[orgID+"|"+projectID] = true
}

type fakeOrganizationChecker struct {
	mu       sync.Mutex
	archived map[string]bool
}

func newFakeOrganizationChecker() *fakeOrganizationChecker {
	return &fakeOrganizationChecker{archived: map[string]bool{}}
}

func (f *fakeOrganizationChecker) IsArchived(ctx context.Context, organizationID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.archived[organizationID], nil
}
