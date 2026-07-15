package application_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"platform-of-platform/internal/audit/domain"
)

type fakeAuditEntryRepo struct {
	mu        sync.Mutex
	entries   []*domain.Entry
	lastLimit int
}

func newFakeAuditEntryRepo() *fakeAuditEntryRepo {
	return &fakeAuditEntryRepo{}
}

func (f *fakeAuditEntryRepo) Create(ctx context.Context, entry *domain.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *entry
	f.entries = append(f.entries, &cp)
	return nil
}

// ListByOrganization mirrors the real keyset semantics closely enough
// for the pagination tests: newest first, strictly before the given
// cursor when one is supplied.
func (f *fakeAuditEntryRepo) ListByOrganization(ctx context.Context, organizationID string, limit int, beforeCreatedAt *time.Time, beforeID *string) ([]*domain.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastLimit = limit

	var matching []*domain.Entry
	for _, e := range f.entries {
		if e.OrganizationID != organizationID {
			continue
		}
		if beforeCreatedAt != nil {
			if e.CreatedAt.After(*beforeCreatedAt) {
				continue
			}
			if e.CreatedAt.Equal(*beforeCreatedAt) && beforeID != nil && e.ID >= *beforeID {
				continue
			}
		}
		cp := *e
		matching = append(matching, &cp)
	}
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].CreatedAt.Equal(matching[j].CreatedAt) {
			return matching[i].ID > matching[j].ID
		}
		return matching[i].CreatedAt.After(matching[j].CreatedAt)
	})
	if len(matching) > limit {
		matching = matching[:limit]
	}
	return matching, nil
}

func (f *fakeAuditEntryRepo) put(e *domain.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	f.entries = append(f.entries, &cp)
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
