package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"platform-of-platform/internal/audit/application"
	"platform-of-platform/internal/audit/domain"
)

const testOrgID = "org-1"

func entryAt(t *testing.T, at time.Time, id string) *domain.Entry {
	t.Helper()
	e := domain.NewEntry(testOrgID, "event-"+id, "user-1", "SomeAction", "workspace", "ws-1", map[string]any{"actor": "user-1"})
	e.ID = id
	e.CreatedAt = at
	return e
}

func TestListAuditEntriesService_RequiresOrganizationManage(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	svc := application.NewListAuditEntriesService(repo, newFakePermissionChecker())

	_, err := svc.Execute(context.Background(), application.ListAuditEntriesInput{
		OrganizationID: testOrgID, RequestingUserID: "member-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}
}

func TestListAuditEntriesService_RejectsMalformedCursor(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	svc := application.NewListAuditEntriesService(repo, perm)

	_, err := svc.Execute(context.Background(), application.ListAuditEntriesInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Cursor: "not-a-valid-cursor!!",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for a malformed cursor, got: %v", err)
	}
}

func TestListAuditEntriesService_LimitIsClampedToMax(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	svc := application.NewListAuditEntriesService(repo, perm)

	if _, err := svc.Execute(context.Background(), application.ListAuditEntriesInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Limit: 100000,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// The service fetches limit+1 to detect a next page - 500 is the
	// documented cap (maxPageLimit).
	if repo.lastLimit != 501 {
		t.Errorf("expected the requested limit to be clamped to the 500 max (+1 lookahead row), got %d", repo.lastLimit)
	}
}

func TestListAuditEntriesService_PaginatesWithACursor(t *testing.T) {
	repo := newFakeAuditEntryRepo()
	perm := newFakePermissionChecker()
	perm.grant(testOrgID, "admin-1", "organization:manage")
	svc := application.NewListAuditEntriesService(repo, perm)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		repo.put(entryAt(t, base.Add(time.Duration(i)*time.Minute), string(rune('a'+i))))
	}

	firstPage, err := svc.Execute(context.Background(), application.ListAuditEntriesInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Limit: 2,
	})
	if err != nil {
		t.Fatalf("Execute (page 1): %v", err)
	}
	if len(firstPage.Entries) != 2 {
		t.Fatalf("expected 2 entries on the first page, got %d", len(firstPage.Entries))
	}
	if firstPage.Entries[0].ID != "c" || firstPage.Entries[1].ID != "b" {
		t.Fatalf("expected newest-first ordering [c, b], got [%s, %s]", firstPage.Entries[0].ID, firstPage.Entries[1].ID)
	}
	if firstPage.NextCursor == "" {
		t.Fatal("expected a NextCursor since a third entry remains")
	}

	secondPage, err := svc.Execute(context.Background(), application.ListAuditEntriesInput{
		OrganizationID: testOrgID, RequestingUserID: "admin-1", Limit: 2, Cursor: firstPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("Execute (page 2): %v", err)
	}
	if len(secondPage.Entries) != 1 || secondPage.Entries[0].ID != "a" {
		t.Fatalf("expected exactly the remaining entry [a] on the second page, got %+v", secondPage.Entries)
	}
	if secondPage.NextCursor != "" {
		t.Errorf("expected no NextCursor on the last page, got %q", secondPage.NextCursor)
	}
}
