package application_test

import (
	"context"
	"sync"

	"platform-of-platform/internal/rbac/domain"
)

type fakeRoleRepo struct {
	mu    sync.Mutex
	roles map[string]*domain.Role
}

func newFakeRoleRepo() *fakeRoleRepo { return &fakeRoleRepo{roles: map[string]*domain.Role{}} }

func (f *fakeRoleRepo) Create(ctx context.Context, role *domain.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	if !ok {
		return nil, domain.ErrRoleNotFound
	}
	cp := *r
	return &cp, nil
}

// put inserts a role bypassing the org-visibility filtering GetByID
// would otherwise apply - test setup only.
func (f *fakeRoleRepo) put(r *domain.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	f.roles[r.ID] = &cp
}

type fakeRoleBindingRepo struct {
	mu       sync.Mutex
	bindings []*domain.RoleBinding
}

func newFakeRoleBindingRepo() *fakeRoleBindingRepo { return &fakeRoleBindingRepo{} }

func (f *fakeRoleBindingRepo) Create(ctx context.Context, b *domain.RoleBinding) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *b
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

type fakeResourceChecker struct {
	mu     sync.Mutex
	exists map[string]bool
}

func newFakeResourceChecker() *fakeResourceChecker {
	return &fakeResourceChecker{exists: map[string]bool{}}
}

func (f *fakeResourceChecker) ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error) {
	return f.check(organizationID, projectID), nil
}

func (f *fakeResourceChecker) Exists(ctx context.Context, organizationID, workspaceID string) (bool, error) {
	return f.check(organizationID, workspaceID), nil
}

func (f *fakeResourceChecker) TeamExists(ctx context.Context, organizationID, teamID string) (bool, error) {
	return f.check(organizationID, teamID), nil
}

func (f *fakeResourceChecker) ServiceAccountExists(ctx context.Context, organizationID, serviceAccountID string) (bool, error) {
	return f.check(organizationID, serviceAccountID), nil
}

func (f *fakeResourceChecker) check(organizationID, id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exists[organizationID+"|"+id]
}

func (f *fakeResourceChecker) add(orgID, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exists[orgID+"|"+id] = true
}
