package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	tenancypg "platform-of-platform/internal/tenancy/adapters/postgres"
	"platform-of-platform/internal/tenancy/domain"
)

func TestOrganizationRepository_CreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := tenancypg.NewOrganizationRepository(pool)
	actorID := insertUser(t, root)

	org, err := domain.NewOrganization("Adapter Test Org", "adapter-org-"+uuid.NewString()[:8])
	if err != nil {
		t.Fatalf("NewOrganization: %v", err)
	}
	if err := repo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, org.ID)
	})

	got, err := repo.GetByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != org.Name || got.Slug != org.Slug {
		t.Errorf("expected name/slug %q/%q, got %q/%q", org.Name, org.Slug, got.Name, got.Slug)
	}
	if got.Status != domain.OrganizationStatusActive {
		t.Errorf("expected a freshly created org to be active, got %q", got.Status)
	}
	if got.ArchivedAt != nil {
		t.Errorf("expected ArchivedAt to be nil, got %v", got.ArchivedAt)
	}

	// Create also writes a real OrganizationCreated event in the same
	// transaction (the Transactional Outbox pattern) - verify the row
	// actually exists, not just that Create returned no error.
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'OrganizationCreated'`, org.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected exactly 1 OrganizationCreated event, got %d", eventCount)
	}
}

func TestOrganizationRepository_GetByID_UnknownReturnsNotFound(t *testing.T) {
	pool := dbtest.AppPool(t)
	repo := tenancypg.NewOrganizationRepository(pool)

	_, err := repo.GetByID(context.Background(), uuid.NewString())
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for an unknown id, got: %v", err)
	}
}

func TestOrganizationRepository_ArchiveAndIsArchived(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := tenancypg.NewOrganizationRepository(pool)
	actorID := insertUser(t, root)

	org, _ := domain.NewOrganization("Archive Test Org", "archive-org-"+uuid.NewString()[:8])
	if err := repo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, org.ID)
	})

	archived, err := repo.IsArchived(ctx, org.ID)
	if err != nil {
		t.Fatalf("IsArchived (before): %v", err)
	}
	if archived {
		t.Fatal("expected a freshly created org not to be archived")
	}

	if err := org.Archive(); err != nil {
		t.Fatalf("domain Archive: %v", err)
	}
	if err := repo.Archive(ctx, org, actorID); err != nil {
		t.Fatalf("repo Archive: %v", err)
	}

	archived, err = repo.IsArchived(ctx, org.ID)
	if err != nil {
		t.Fatalf("IsArchived (after): %v", err)
	}
	if !archived {
		t.Error("expected the org to be archived after Archive()")
	}

	got, err := repo.GetByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ArchivedAt == nil {
		t.Error("expected ArchivedAt to be set after archiving")
	}

	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE organization_id = $1 AND event_type = 'OrganizationArchived'`, org.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected exactly 1 OrganizationArchived event, got %d", eventCount)
	}
}

func TestOrganizationRepository_IsArchived_UnknownReturnsNotFound(t *testing.T) {
	pool := dbtest.AppPool(t)
	repo := tenancypg.NewOrganizationRepository(pool)

	_, err := repo.IsArchived(context.Background(), uuid.NewString())
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for an unknown id, got: %v", err)
	}
}

// TestOrganizationRepository_FindOrganizationsPastPurgeWindow is the
// real regression test for the RLS bug this method's own doc comment
// describes finding for real ("the first version of this method queried
// `organizations` directly and the reaper never purged anything") -
// proves the outbox_events-based cross-org scan actually works, and
// respects the archivedBefore cutoff (an org archived "now" must not
// show up when the reaper asks for orgs archived before an hour ago).
func TestOrganizationRepository_FindOrganizationsPastPurgeWindow(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	repo := tenancypg.NewOrganizationRepository(pool)
	actorID := insertUser(t, root)

	pastWindowOrg, _ := domain.NewOrganization("Past Window Org", "past-window-org-"+uuid.NewString()[:8])
	if err := repo.Create(ctx, pastWindowOrg, actorID); err != nil {
		t.Fatalf("Create pastWindowOrg: %v", err)
	}
	_ = pastWindowOrg.Archive()
	if err := repo.Archive(ctx, pastWindowOrg, actorID); err != nil {
		t.Fatalf("Archive pastWindowOrg: %v", err)
	}
	// Backdate the OrganizationArchived event so it looks like it
	// happened well outside any real grace window, without waiting for
	// real wall-clock time to pass.
	mustExec(t, root, `UPDATE outbox_events SET occurred_at = now() - interval '48 hours' WHERE organization_id = $1 AND event_type = 'OrganizationArchived'`, pastWindowOrg.ID)

	recentlyArchivedOrg, _ := domain.NewOrganization("Recently Archived Org", "recent-org-"+uuid.NewString()[:8])
	if err := repo.Create(ctx, recentlyArchivedOrg, actorID); err != nil {
		t.Fatalf("Create recentlyArchivedOrg: %v", err)
	}
	_ = recentlyArchivedOrg.Archive()
	if err := repo.Archive(ctx, recentlyArchivedOrg, actorID); err != nil {
		t.Fatalf("Archive recentlyArchivedOrg: %v", err)
	}

	t.Cleanup(func() {
		for _, id := range []string{pastWindowOrg.ID, recentlyArchivedOrg.ID} {
			mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, id)
			mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, id)
			mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, id)
		}
	})

	cutoff := time.Now().Add(-24 * time.Hour)
	ids, err := repo.FindOrganizationsPastPurgeWindow(ctx, cutoff)
	if err != nil {
		t.Fatalf("FindOrganizationsPastPurgeWindow: %v", err)
	}

	var foundPast, foundRecent bool
	for _, id := range ids {
		if id == pastWindowOrg.ID {
			foundPast = true
		}
		if id == recentlyArchivedOrg.ID {
			foundRecent = true
		}
	}
	if !foundPast {
		t.Error("expected the backdated (48h ago) archived org to be found")
	}
	if foundRecent {
		t.Error("expected the just-archived org NOT to be found (still inside the grace window)")
	}
}

// TestOrganizationRepository_Purge is the real regression test for
// Purge's own doc comment: "without app.current_org_id set, platform_app
// can't see or delete a single row in any of them, RLS silently narrows
// every DELETE to zero rows instead of erroring." Creates a real org
// with a real project hanging off it, purges, and proves both rows are
// actually gone - not just that Purge returned no error.
func TestOrganizationRepository_Purge(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	projectRepo := tenancypg.NewProjectRepository(pool)
	actorID := insertUser(t, root)

	org, _ := domain.NewOrganization("Purge Test Org", "purge-org-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	project, _ := domain.NewProject(org.ID, "Purge Test Project", "purge-project", "")
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create project: %v", err)
	}
	// Belt-and-braces cleanup in case Purge itself fails partway and the
	// test fails before proving anything - harmless no-ops if Purge
	// already removed these rows.
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM projects WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, org.ID)
	})

	if err := orgRepo.Purge(ctx, org.ID); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	var orgCount, projectCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id = $1`, org.ID).Scan(&orgCount); err != nil {
		t.Fatalf("query organizations: %v", err)
	}
	if err := root.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id = $1`, project.ID).Scan(&projectCount); err != nil {
		t.Fatalf("query projects: %v", err)
	}
	if orgCount != 0 {
		t.Error("expected the organization row to be gone after Purge")
	}
	if projectCount != 0 {
		t.Error("expected the project row to be gone after Purge")
	}

	// Purge deliberately never touches users (platform-global, not
	// org-scoped) - the actor must still exist.
	var userCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, actorID).Scan(&userCount); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if userCount != 1 {
		t.Error("expected Purge to leave the actor's own user row untouched")
	}
}
