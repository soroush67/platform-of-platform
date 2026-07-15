package application_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"platform-of-platform/internal/tenancy/application"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakePurgeRepo - a real, hand-written PurgeRepository, independent of
// the real cross-org outbox_events-scanning mechanics the postgres
// adapter needs (internal/platform/dbtest's own integration test covers
// that RLS/lookup subtlety for real) - this unit test is purely about
// PurgeReaperService's own control flow: does it call Purge for exactly
// the candidates FindOrganizationsPastPurgeWindow returns, and does one
// failing Purge call still let the others proceed.
type fakePurgeRepo struct {
	mu         sync.Mutex
	pastWindow []string
	purged     []string
	failFor    string
}

func (f *fakePurgeRepo) FindOrganizationsPastPurgeWindow(ctx context.Context, archivedBefore time.Time) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.pastWindow))
	copy(out, f.pastWindow)
	return out, nil
}

// Purge removes organizationID from pastWindow on success - the real
// postgres adapter's own Purge deletes the outbox_events row
// FindOrganizationsPastPurgeWindow scans, so a real subsequent sweep
// naturally stops finding an already-purged org; this fake mirrors that
// so a real ticker firing more than once during a test doesn't
// re-"purge" the same organization repeatedly.
func (f *fakePurgeRepo) Purge(ctx context.Context, organizationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if organizationID == f.failFor {
		return context.DeadlineExceeded
	}
	f.purged = append(f.purged, organizationID)
	for i, id := range f.pastWindow {
		if id == organizationID {
			f.pastWindow = append(f.pastWindow[:i], f.pastWindow[i+1:]...)
			break
		}
	}
	return nil
}

func TestPurgeReaperService_PurgesEveryCandidateFromOneSweep(t *testing.T) {
	repo := &fakePurgeRepo{pastWindow: []string{"org-a", "org-b", "org-c"}}
	svc := application.NewPurgeReaperService(repo, 720*time.Hour, 10*time.Millisecond, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = svc.Run(ctx)

	if len(repo.purged) != 3 {
		t.Fatalf("expected all 3 candidates purged, got %v", repo.purged)
	}
}

func TestPurgeReaperService_OneFailureDoesNotBlockTheRest(t *testing.T) {
	repo := &fakePurgeRepo{pastWindow: []string{"org-a", "org-b", "org-c"}, failFor: "org-b"}
	svc := application.NewPurgeReaperService(repo, 720*time.Hour, 10*time.Millisecond, discardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = svc.Run(ctx)

	found := map[string]bool{}
	for _, id := range repo.purged {
		found[id] = true
	}
	if !found["org-a"] || !found["org-c"] {
		t.Errorf("expected org-a and org-c to still be purged despite org-b failing, got %v", repo.purged)
	}
	if found["org-b"] {
		t.Error("expected org-b to NOT be marked purged (its Purge call failed)")
	}
}
