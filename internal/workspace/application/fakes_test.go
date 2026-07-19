package application_test

import (
	"context"
	"sync"

	"platform-of-platform/internal/workspace/domain"
)

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
// (project_visibility.go) - deliberately separate from
// fakePermissionChecker above (a grant here is scoped to an exact
// (permission, scopeType, scopeID) tuple, no organization-scope
// fallback, matching the real HasScopedPermission's own narrower
// contract).
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

func (f *fakeOrganizationChecker) archive(orgID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.archived[orgID] = true
}
