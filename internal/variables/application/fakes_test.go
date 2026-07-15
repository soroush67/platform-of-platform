package application_test

import (
	"context"
	"sync"

	"platform-of-platform/internal/variables/domain"
)

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

func (f *fakeProjectChecker) add(orgID, projectID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.projects[orgID+"|"+projectID] = true
}

type fakeEnvironmentChecker struct {
	mu           sync.Mutex
	environments map[string]bool
}

func newFakeEnvironmentChecker() *fakeEnvironmentChecker {
	return &fakeEnvironmentChecker{environments: map[string]bool{}}
}

func (f *fakeEnvironmentChecker) Exists(ctx context.Context, organizationID, environmentID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.environments[organizationID+"|"+environmentID], nil
}

func (f *fakeEnvironmentChecker) add(orgID, environmentID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.environments[orgID+"|"+environmentID] = true
}

type fakeWorkspaceChecker struct {
	mu     sync.Mutex
	scopes map[string]struct {
		projectID     string
		environmentID *string
	}
}

func newFakeWorkspaceChecker() *fakeWorkspaceChecker {
	return &fakeWorkspaceChecker{scopes: map[string]struct {
		projectID     string
		environmentID *string
	}{}}
}

func (f *fakeWorkspaceChecker) Exists(ctx context.Context, organizationID, workspaceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.scopes[organizationID+"|"+workspaceID]
	return ok, nil
}

func (f *fakeWorkspaceChecker) GetScope(ctx context.Context, organizationID, workspaceID string) (string, *string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.scopes[organizationID+"|"+workspaceID]
	if !ok {
		return "", nil, domain.ErrScopeNotFound
	}
	return s.projectID, s.environmentID, nil
}

func (f *fakeWorkspaceChecker) add(orgID, workspaceID, projectID string, environmentID *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes[orgID+"|"+workspaceID] = struct {
		projectID     string
		environmentID *string
	}{projectID: projectID, environmentID: environmentID}
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
