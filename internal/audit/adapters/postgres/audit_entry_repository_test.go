package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"platform-of-platform/internal/audit/adapters/postgres"
	"platform-of-platform/internal/audit/domain"
	"platform-of-platform/internal/platform/dbtest"
)

func TestAuditEntryRepository_CreateAndListByOrganization(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAuditEntryRepository(pool)
	orgID := insertOrg(t, root)

	entry := domain.NewEntry(orgID, uuid.NewString(), "user-1", "OrganizationCreated", "organization", orgID, map[string]any{"name": "Acme"})
	if err := repo.Create(ctx, entry); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.ListByOrganization(ctx, orgID, 10, nil, nil)
	if err != nil {
		t.Fatalf("ListByOrganization: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 entry, got %d", len(got))
	}
	if got[0].Actor != "user-1" || got[0].Action != "OrganizationCreated" || got[0].TargetID != orgID {
		t.Errorf("expected fields to round-trip, got %+v", got[0])
	}
	if got[0].Metadata["name"] != "Acme" {
		t.Errorf("expected metadata to round-trip through the jsonb column, got %v", got[0].Metadata)
	}
}

// TestAuditEntryRepository_Create_RedeliveredEventIsANoOp is the real
// regression test for Create's own ON CONFLICT (source_event_id) DO
// NOTHING - migrations/0008_audit_idempotency.up.sql's whole reason to
// exist: the Outbox Relay's at-least-once redelivery must not turn one
// real event into two audit rows.
func TestAuditEntryRepository_Create_RedeliveredEventIsANoOp(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAuditEntryRepository(pool)
	orgID := insertOrg(t, root)

	sourceEventID := uuid.NewString()
	first := domain.NewEntry(orgID, sourceEventID, "user-1", "OrganizationCreated", "organization", orgID, map[string]any{})
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create (first): %v", err)
	}

	// A redelivery of the same underlying outbox event - a fresh entry
	// id, but the same source_event_id.
	redelivered := domain.NewEntry(orgID, sourceEventID, "user-1", "OrganizationCreated", "organization", orgID, map[string]any{})
	if err := repo.Create(ctx, redelivered); err != nil {
		t.Fatalf("Create (redelivered): %v", err)
	}

	var count int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM audit_entries WHERE source_event_id = $1`, sourceEventID).Scan(&count); err != nil {
		t.Fatalf("query audit_entries: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row despite two Create calls with the same source_event_id, got %d", count)
	}
}

// TestAuditEntryRepository_ListByOrganization_KeysetPagination is the
// real regression test for the row-value cursor comparison
// ListByOrganization's own doc comment describes: "(created_at, id) <
// ($2, $3)... no entry is ever skipped or repeated across a page
// boundary," specifically the case that comparison exists for - several
// entries sharing the exact same created_at.
func TestAuditEntryRepository_ListByOrganization_KeysetPagination(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := postgres.NewAuditEntryRepository(pool)
	orgID := insertOrg(t, root)

	sameInstant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := 0; i < 5; i++ {
		e := domain.NewEntry(orgID, uuid.NewString(), "user-1", "Action", "thing", uuid.NewString(), map[string]any{})
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		// Force every entry to the exact same created_at - the specific
		// tie-breaking case a naive "created_at < cursor" comparison
		// (without also comparing id) would get wrong.
		mustExec(t, root, `UPDATE audit_entries SET created_at = $1 WHERE id = $2`, sameInstant, e.ID)
		ids = append(ids, e.ID)
	}

	seen := map[string]bool{}
	var beforeCreatedAt *time.Time
	var beforeID *string
	for page := 0; page < 10; page++ {
		got, err := repo.ListByOrganization(ctx, orgID, 2, beforeCreatedAt, beforeID)
		if err != nil {
			t.Fatalf("ListByOrganization (page %d): %v", page, err)
		}
		if len(got) == 0 {
			break
		}
		for _, e := range got {
			if seen[e.ID] {
				t.Fatalf("entry %s was returned on more than one page - the keyset cursor repeated a row", e.ID)
			}
			seen[e.ID] = true
		}
		last := got[len(got)-1]
		beforeCreatedAt = &last.CreatedAt
		beforeID = &last.ID
	}

	if len(seen) != len(ids) {
		t.Errorf("expected all %d entries to be seen exactly once across pages, saw %d", len(ids), len(seen))
	}
}
