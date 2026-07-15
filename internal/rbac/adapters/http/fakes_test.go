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
	"platform-of-platform/internal/rbac/domain"
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

type fakeRoleRepo struct {
	mu    sync.Mutex
	roles map[string]*domain.Role
}

func newFakeRoleRepo() *fakeRoleRepo { return &fakeRoleRepo{roles: map[string]*domain.Role{}} }

func (f *fakeRoleRepo) Create(ctx context.Context, role *domain.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.roles {
		if r.OrganizationID != nil && role.OrganizationID != nil && *r.OrganizationID == *role.OrganizationID && r.Name == role.Name {
			return domain.ErrRoleAlreadyExists
		}
	}
	cp := *role
	f.roles[role.ID] = &cp
	return nil
}

func (f *fakeRoleRepo) ListForOrganization(ctx context.Context, organizationID string) ([]*domain.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Role
	for _, r := range f.roles {
		if r.OrganizationID == nil || *r.OrganizationID == organizationID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeRoleRepo) GetByID(ctx context.Context, organizationID, roleID string) (*domain.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.roles[roleID]
	if !ok || (r.OrganizationID != nil && *r.OrganizationID != organizationID) {
		return nil, domain.ErrRoleNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRoleRepo) put(role *domain.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *role
	f.roles[role.ID] = &cp
}

type fakeRoleBindingRepo struct {
	mu       sync.Mutex
	bindings []*domain.RoleBinding
}

func newFakeRoleBindingRepo() *fakeRoleBindingRepo { return &fakeRoleBindingRepo{} }

func (f *fakeRoleBindingRepo) Create(ctx context.Context, binding *domain.RoleBinding) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *binding
	f.bindings = append(f.bindings, &cp)
	return nil
}

func (f *fakeRoleBindingRepo) ListForSubject(ctx context.Context, organizationID, subjectID string) ([]*domain.RoleBinding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.RoleBinding
	for _, b := range f.bindings {
		if b.OrganizationID != organizationID {
			continue
		}
		if subjectID != "" && b.SubjectID != subjectID {
			continue
		}
		cp := *b
		out = append(out, &cp)
	}
	return out, nil
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

// fakeResourceChecker satisfies ProjectChecker/WorkspaceChecker/
// TeamChecker/ServiceAccountChecker at once via a single generic
// exists set - same combined-fake pattern already used for the RBAC
// application-service unit tests earlier this session.
type fakeResourceChecker struct {
	mu     sync.Mutex
	exists map[string]bool
}

func newFakeResourceChecker() *fakeResourceChecker {
	return &fakeResourceChecker{exists: map[string]bool{}}
}

func (f *fakeResourceChecker) ProjectExists(ctx context.Context, organizationID, id string) (bool, error) {
	return f.has(organizationID, id), nil
}
func (f *fakeResourceChecker) Exists(ctx context.Context, organizationID, id string) (bool, error) {
	return f.has(organizationID, id), nil
}
func (f *fakeResourceChecker) TeamExists(ctx context.Context, organizationID, id string) (bool, error) {
	return f.has(organizationID, id), nil
}
func (f *fakeResourceChecker) ServiceAccountExists(ctx context.Context, organizationID, id string) (bool, error) {
	return f.has(organizationID, id), nil
}

func (f *fakeResourceChecker) has(orgID, id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exists[orgID+"|"+id]
}

func (f *fakeResourceChecker) add(orgID, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exists[orgID+"|"+id] = true
}
