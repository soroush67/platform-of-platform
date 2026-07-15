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

	"platform-of-platform/internal/execution/domain"
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

type fakeRunRepo struct {
	mu   sync.Mutex
	runs map[string]*domain.Run
}

func newFakeRunRepo() *fakeRunRepo { return &fakeRunRepo{runs: map[string]*domain.Run{}} }

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
	cp := *run
	f.runs[run.ID] = &cp
	return nil
}

func (f *fakeRunRepo) TryStartApplying(ctx context.Context, organizationID, runID, workspaceID string) (bool, error) {
	return false, nil
}
func (f *fakeRunRepo) RevertToQueued(ctx context.Context, organizationID, runID string) error {
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
	lockedBy map[string]string
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

type fakeWorkerCanceler struct{}

func (f *fakeWorkerCanceler) CancelJob(ctx context.Context, runID string) (bool, error) {
	return true, nil
}
