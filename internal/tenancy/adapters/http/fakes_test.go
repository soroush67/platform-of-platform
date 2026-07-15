package http_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"platform-of-platform/internal/tenancy/domain"

	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
)

func newReader(body []byte) io.Reader { return bytes.NewReader(body) }

var testJWTSecret = []byte("http-adapter-test-secret")

// authedRequest builds a real request that has genuinely passed through
// httpserver.RequireAuth (a real JWT, real parsing) - not a shortcut
// context injection, since userIDContextKey is unexported and this is
// the only real way a request context ever legitimately carries an
// authenticated user id in this codebase.
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

type fakeOrgRepo struct {
	mu   sync.Mutex
	orgs map[string]*domain.Organization
	fail error
}

func newFakeOrgRepo() *fakeOrgRepo { return &fakeOrgRepo{orgs: map[string]*domain.Organization{}} }

func (f *fakeOrgRepo) Create(ctx context.Context, org *domain.Organization, createdByUserID string) error {
	if f.fail != nil {
		return f.fail
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *org
	f.orgs[org.ID] = &cp
	return nil
}

func (f *fakeOrgRepo) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	org, ok := f.orgs[id]
	if !ok {
		return nil, domain.ErrOrganizationNotFound
	}
	cp := *org
	return &cp, nil
}

func (f *fakeOrgRepo) Archive(ctx context.Context, org *domain.Organization, archivedByUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.orgs[org.ID]
	if !ok {
		return domain.ErrOrganizationNotFound
	}
	stored.Status = org.Status
	stored.ArchivedAt = org.ArchivedAt
	return nil
}

func (f *fakeOrgRepo) put(org *domain.Organization) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *org
	f.orgs[org.ID] = &cp
}

type fakeMembershipRepo struct {
	mu      sync.Mutex
	members map[string]bool
}

func newFakeMembershipRepo() *fakeMembershipRepo {
	return &fakeMembershipRepo{members: map[string]bool{}}
}

func (f *fakeMembershipRepo) Create(ctx context.Context, m *domain.OrganizationMembership) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[m.OrganizationID+"|"+m.UserID] = true
	return nil
}

func (f *fakeMembershipRepo) IsMember(ctx context.Context, organizationID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[organizationID+"|"+userID], nil
}

func (f *fakeMembershipRepo) add(orgID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[orgID+"|"+userID] = true
}

type fakeRoleAssigner struct{}

func (f *fakeRoleAssigner) AssignRole(ctx context.Context, organizationID, userID, roleName string) error {
	return nil
}

type fakeRoleChanger struct{ mu sync.Mutex }

func (f *fakeRoleChanger) ReplaceRole(ctx context.Context, organizationID, userID, roleName string) error {
	return nil
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

type fakeProjectRepo struct {
	mu       sync.Mutex
	projects map[string]*domain.Project
}

func newFakeProjectRepo() *fakeProjectRepo {
	return &fakeProjectRepo{projects: map[string]*domain.Project{}}
}

func (f *fakeProjectRepo) Create(ctx context.Context, p *domain.Project) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *p
	f.projects[p.ID] = &cp
	return nil
}

func (f *fakeProjectRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.projects[id]
	if !ok || p.OrganizationID != organizationID {
		return nil, domain.ErrProjectNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakeProjectRepo) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Project
	for _, p := range f.projects {
		if p.OrganizationID == organizationID {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeProjectRepo) put(p *domain.Project) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *p
	f.projects[p.ID] = &cp
}

type fakeTeamRepo struct {
	mu          sync.Mutex
	teams       map[string]*domain.Team
	memberships map[string]bool
}

func newFakeTeamRepo() *fakeTeamRepo {
	return &fakeTeamRepo{teams: map[string]*domain.Team{}, memberships: map[string]bool{}}
}

func (f *fakeTeamRepo) Create(ctx context.Context, team *domain.Team) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *team
	f.teams[team.ID] = &cp
	return nil
}

func (f *fakeTeamRepo) AddMember(ctx context.Context, m *domain.TeamMembership) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.memberships[m.TeamID+"|"+m.UserID] = true
	return nil
}

func (f *fakeTeamRepo) RemoveMember(ctx context.Context, organizationID, teamID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.memberships, teamID+"|"+userID)
	return nil
}

func (f *fakeTeamRepo) GetByID(ctx context.Context, organizationID, teamID string) (*domain.Team, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	team, ok := f.teams[teamID]
	if !ok || team.OrganizationID != organizationID {
		return nil, domain.ErrTeamNotFound
	}
	cp := *team
	return &cp, nil
}

func (f *fakeTeamRepo) put(team *domain.Team) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *team
	f.teams[team.ID] = &cp
}
