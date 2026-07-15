package application_test

// In-memory fakes for every port this context's own application
// services declare (ports.go, create_team.go, archive_organization.go,
// purge_reaper.go) - real, hand-written implementations, not a mocking
// framework: each service's ports are already narrow (dependency
// inversion per docs/architecture/18-backend-structure.md §3), so a
// fake is a handful of lines, and using real Go types/control flow
// instead of a mock-expectation DSL keeps these tests reading like the
// behavior they verify, not like mock setup boilerplate.

import (
	"context"
	"sync"

	"platform-of-platform/internal/tenancy/domain"
)

type fakeOrgRepo struct {
	mu   sync.Mutex
	orgs map[string]*domain.Organization
}

func newFakeOrgRepo() *fakeOrgRepo { return &fakeOrgRepo{orgs: map[string]*domain.Organization{}} }

func (f *fakeOrgRepo) Create(ctx context.Context, org *domain.Organization, createdByUserID string) error {
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

// Archive satisfies ArchiveOrganizationRepository - the same fake plays
// both roles, same as the real rbac postgres adapter's own "one
// concrete type satisfies several ports" pattern.
func (f *fakeOrgRepo) Archive(ctx context.Context, org *domain.Organization, archivedByUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *org
	f.orgs[org.ID] = &cp
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
	members map[string]bool // "orgID|userID" -> true
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

// fakePermChecker: a permission is granted per (orgID, userID) pair -
// set explicit permission sets per test rather than modeling full RBAC
// evaluation, which internal/rbac's own real tests already cover.
type fakePermChecker struct {
	mu    sync.Mutex
	perms map[string]map[string]bool // "orgID|userID" -> set of permissions
}

func newFakePermChecker() *fakePermChecker {
	return &fakePermChecker{perms: map[string]map[string]bool{}}
}

func (f *fakePermChecker) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	set := f.perms[organizationID+"|"+userID]
	return set != nil && set[permission], nil
}

func (f *fakePermChecker) grant(orgID, userID, permission string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := orgID + "|" + userID
	if f.perms[key] == nil {
		f.perms[key] = map[string]bool{}
	}
	f.perms[key][permission] = true
}

// fakeRoleAssigner satisfies both RoleAssigner and RoleChanger -
// records every call so tests can assert on what role a subject ended
// up with, the same "spy" shape a real Role*Service verification needs.
type fakeRoleAssigner struct {
	mu       sync.Mutex
	assigned map[string]string // "orgID|userID" -> roleName (last write wins, same as ReplaceRole)
	err      error
}

func newFakeRoleAssigner() *fakeRoleAssigner {
	return &fakeRoleAssigner{assigned: map[string]string{}}
}

func (f *fakeRoleAssigner) AssignRole(ctx context.Context, organizationID, userID, roleName string) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assigned[organizationID+"|"+userID] = roleName
	return nil
}

func (f *fakeRoleAssigner) ReplaceRole(ctx context.Context, organizationID, userID, roleName string) error {
	return f.AssignRole(ctx, organizationID, userID, roleName)
}

func (f *fakeRoleAssigner) roleOf(orgID, userID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assigned[orgID+"|"+userID]
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

type fakeTeamRepo struct {
	mu          sync.Mutex
	teams       map[string]*domain.Team
	memberships map[string]bool // "teamID|userID"
}

func newFakeTeamRepo() *fakeTeamRepo {
	return &fakeTeamRepo{teams: map[string]*domain.Team{}, memberships: map[string]bool{}}
}

func (f *fakeTeamRepo) Create(ctx context.Context, t *domain.Team) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.teams[t.ID] = &cp
	return nil
}

func (f *fakeTeamRepo) GetByID(ctx context.Context, organizationID, teamID string) (*domain.Team, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.teams[teamID]
	if !ok || t.OrganizationID != organizationID {
		return nil, domain.ErrTeamNotFound
	}
	cp := *t
	return &cp, nil
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

func (f *fakeTeamRepo) isMember(teamID, userID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.memberships[teamID+"|"+userID]
}
