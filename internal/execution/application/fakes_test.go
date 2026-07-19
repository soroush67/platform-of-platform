package application_test

import (
	"context"
	"sync"
	"time"

	"platform-of-platform/internal/execution/domain"
)

type fakeRunRepo struct {
	mu       sync.Mutex
	runs     map[string]*domain.Run
	locker   *fakeWorkspaceLocker // TryStartApplying needs to know who currently holds the lock, same as the real adapter's single "runs + locked_by" invariant.
	failMark string
}

func newFakeRunRepo(locker *fakeWorkspaceLocker) *fakeRunRepo {
	return &fakeRunRepo{runs: map[string]*domain.Run{}, locker: locker}
}

func (f *fakeRunRepo) Create(ctx context.Context, run *domain.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *run
	f.runs[run.ID] = &cp
	return nil
}

func (f *fakeRunRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[id]
	if !ok || r.OrganizationID != organizationID {
		return nil, domain.ErrRunNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRunRepo) ListByWorkspace(ctx context.Context, organizationID, workspaceID string) ([]*domain.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Run
	for _, r := range f.runs {
		if r.OrganizationID == organizationID && r.WorkspaceID == workspaceID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeRunRepo) Update(ctx context.Context, run *domain.Run, actorUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.runs[run.ID]; !ok {
		return domain.ErrRunNotFound
	}
	cp := *run
	f.runs[run.ID] = &cp
	return nil
}

func (f *fakeRunRepo) TryStartApplying(ctx context.Context, organizationID, runID, workspaceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[runID]
	if !ok || r.OrganizationID != organizationID || r.Status != domain.RunStatusQueued {
		return false, nil
	}
	now := time.Now().UTC()
	r.Status = domain.RunStatusApplying
	r.StartedAt = &now
	return true, nil
}

func (f *fakeRunRepo) RevertToQueued(ctx context.Context, organizationID, runID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[runID]
	if !ok || r.OrganizationID != organizationID {
		return domain.ErrRunNotFound
	}
	r.Status = domain.RunStatusQueued
	r.StartedAt = nil
	return nil
}

func (f *fakeRunRepo) FindStaleApplyingRuns(ctx context.Context, olderThan time.Time) ([]domain.StaleRunCandidate, error) {
	return nil, nil
}

func (f *fakeRunRepo) MarkErroredIfStillApplying(ctx context.Context, organizationID, runID string) (bool, error) {
	return false, nil
}

func (f *fakeRunRepo) put(run *domain.Run) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *run
	f.runs[run.ID] = &cp
}

type fakeWorkspaceLocker struct {
	mu       sync.Mutex
	lockedBy map[string]string // workspaceID -> runID
}

func newFakeWorkspaceLocker() *fakeWorkspaceLocker {
	return &fakeWorkspaceLocker{lockedBy: map[string]string{}}
}

func (f *fakeWorkspaceLocker) TryLock(ctx context.Context, organizationID, workspaceID, runID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, locked := f.lockedBy[workspaceID]; locked {
		return false, nil
	}
	f.lockedBy[workspaceID] = runID
	return true, nil
}

func (f *fakeWorkspaceLocker) Unlock(ctx context.Context, organizationID, workspaceID, runID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lockedBy[workspaceID] == runID {
		delete(f.lockedBy, workspaceID)
	}
	return nil
}

func (f *fakeWorkspaceLocker) isLocked(workspaceID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, locked := f.lockedBy[workspaceID]
	return locked
}

type fakeWorkspaceChecker struct {
	mu         sync.Mutex
	workspaces map[string]bool
}

func newFakeWorkspaceChecker() *fakeWorkspaceChecker {
	return &fakeWorkspaceChecker{workspaces: map[string]bool{}}
}

func (f *fakeWorkspaceChecker) WorkspaceExists(ctx context.Context, organizationID, projectID, workspaceID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.workspaces[organizationID+"|"+projectID+"|"+workspaceID], nil
}

func (f *fakeWorkspaceChecker) add(orgID, projectID, workspaceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workspaces[orgID+"|"+projectID+"|"+workspaceID] = true
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

type fakeScopedPermissionChecker struct {
	mu    sync.Mutex
	perms map[string]bool
}

func newFakeScopedPermissionChecker() *fakeScopedPermissionChecker {
	return &fakeScopedPermissionChecker{perms: map[string]bool{}}
}

func (f *fakeScopedPermissionChecker) HasPermissionAtScope(ctx context.Context, organizationID, userID, permission string, projectID, workspaceID *string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perms[organizationID+"|"+userID+"|"+permission], nil
}

func (f *fakeScopedPermissionChecker) grant(orgID, userID, permission string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.perms[orgID+"|"+userID+"|"+permission] = true
}

// fakePermissionChecker/fakeVisibilityChecker back the new
// PermissionChecker/VisibilityChecker ports (project_visibility.go) -
// ListRunsService/GetRunService's own visibility gate, separate from
// fakeScopedPermissionChecker above (which backs TriggerRunService/
// CancelRunService's existing workspace:apply check and has no real
// bearing on this new gate).
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

type fakeWorkspaceEngineReader struct {
	mu      sync.Mutex
	engines map[string]string
}

func newFakeWorkspaceEngineReader() *fakeWorkspaceEngineReader {
	return &fakeWorkspaceEngineReader{engines: map[string]string{}}
}

func (f *fakeWorkspaceEngineReader) GetExecutionEngine(ctx context.Context, organizationID, workspaceID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.engines[organizationID+"|"+workspaceID], nil
}

func (f *fakeWorkspaceEngineReader) set(orgID, workspaceID, engine string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engines[orgID+"|"+workspaceID] = engine
}

type fakeVariableResolver struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeVariableResolver() *fakeVariableResolver {
	return &fakeVariableResolver{values: map[string]string{}}
}

func (f *fakeVariableResolver) ResolveValue(ctx context.Context, organizationID, workspaceID, key, requestingUserID string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[organizationID+"|"+workspaceID+"|"+key]
	return v, ok, nil
}

func (f *fakeVariableResolver) set(orgID, workspaceID, key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[orgID+"|"+workspaceID+"|"+key] = value
}

type fakeWorkerDispatcher struct {
	mu                   sync.Mutex
	dispatched           bool
	calls                int
	lastConfigBundle     string
	lastCredentialBundle string
}

func newFakeWorkerDispatcher(dispatched bool) *fakeWorkerDispatcher {
	return &fakeWorkerDispatcher{dispatched: dispatched}
}

func (f *fakeWorkerDispatcher) Dispatch(ctx context.Context, runID, organizationID, workspaceID, executionEngine, configBundle, credentialBundle string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastConfigBundle = configBundle
	f.lastCredentialBundle = credentialBundle
	return f.dispatched, nil
}

type fakeWorkerCanceler struct {
	mu       sync.Mutex
	canceled []string
}

func newFakeWorkerCanceler() *fakeWorkerCanceler {
	return &fakeWorkerCanceler{}
}

func (f *fakeWorkerCanceler) CancelJob(ctx context.Context, runID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.canceled = append(f.canceled, runID)
	return true, nil
}

type fakeRunTracker struct {
	mu        sync.Mutex
	forgotten map[string]bool
}

func newFakeRunTracker() *fakeRunTracker {
	return &fakeRunTracker{forgotten: map[string]bool{}}
}

func (f *fakeRunTracker) Forget(runID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgotten[runID] = true
}

func (f *fakeRunTracker) wasForgotten(runID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forgotten[runID]
}
