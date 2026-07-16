package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	tenancypg "platform-of-platform/internal/tenancy/adapters/postgres"
	"platform-of-platform/internal/tenancy/domain"
)

func TestRootMembershipRepository_ReturnsEveryOrgForTheUserOrderedByCreatedAt(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	membershipRepo := tenancypg.NewMembershipRepository(pool)
	rootMembershipRepo := tenancypg.NewRootMembershipRepository(root)

	actorID := insertUser(t, root)
	orgA, _ := domain.NewOrganization("Root Membership Org A", "root-membership-a-"+uuid.NewString()[:8])
	orgB, _ := domain.NewOrganization("Root Membership Org B", "root-membership-b-"+uuid.NewString()[:8])
	for _, org := range []*domain.Organization{orgA, orgB} {
		if err := orgRepo.Create(ctx, org, actorID); err != nil {
			t.Fatalf("Create org: %v", err)
		}
		if err := membershipRepo.Create(ctx, domain.NewOrganizationMembership(org.ID, actorID)); err != nil {
			t.Fatalf("Create membership: %v", err)
		}
		orgID := org.ID
		t.Cleanup(func() {
			mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, orgID)
			mustExec(t, root, `DELETE FROM organization_memberships WHERE organization_id = $1`, orgID)
			dbtest.DeleteOrganization(t, root, orgID)
		})
	}

	orgs, err := rootMembershipRepo.ListOrganizationsForUser(ctx, actorID)
	if err != nil {
		t.Fatalf("ListOrganizationsForUser: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("expected exactly the 2 orgs this user belongs to, got %d", len(orgs))
	}
	if orgs[0].CreatedAt.After(orgs[1].CreatedAt) {
		t.Error("expected orgs ordered by created_at ascending")
	}
}

// TestRootMembershipRepository_CountOrganizations_ReflectsRealInsert
// asserts a delta, not an absolute count - organizations is shared with
// every other test in this package, so an absolute assertion would be
// flaky by construction (same reasoning as TestUserRepository_Count's
// own doc comment).
func TestRootMembershipRepository_CountOrganizations_ReflectsRealInsert(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	rootMembershipRepo := tenancypg.NewRootMembershipRepository(root)

	actorID := insertUser(t, root)
	before, err := rootMembershipRepo.CountOrganizations(ctx)
	if err != nil {
		t.Fatalf("CountOrganizations (before): %v", err)
	}

	org, _ := domain.NewOrganization("Root Membership Count Org", "root-membership-count-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		dbtest.DeleteOrganization(t, root, org.ID)
	})

	after, err := rootMembershipRepo.CountOrganizations(ctx)
	if err != nil {
		t.Fatalf("CountOrganizations (after): %v", err)
	}
	if after != before+1 {
		t.Errorf("expected CountOrganizations to increase by exactly 1 after a real Create, got before=%d after=%d", before, after)
	}
}

func TestRootMembershipRepository_NoMembershipsReturnsEmptyNotError(t *testing.T) {
	root := dbtest.RootPool(t)
	rootMembershipRepo := tenancypg.NewRootMembershipRepository(root)

	orgs, err := rootMembershipRepo.ListOrganizationsForUser(context.Background(), uuid.NewString())
	if err != nil {
		t.Fatalf("ListOrganizationsForUser: %v", err)
	}
	if len(orgs) != 0 {
		t.Errorf("expected zero orgs for a user with no real memberships, got %d", len(orgs))
	}
}

// TestRootMembershipRepository_NeverReturnsAnotherUsersMemberships is the
// single most important test in this file - it proves the actual
// security property RootMembershipRepository's own doc comment claims
// (safe because the query is always filtered by the caller's own
// user_id) against real RLS-bypassing root, not just asserted in a
// comment.
func TestRootMembershipRepository_NeverReturnsAnotherUsersMemberships(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	membershipRepo := tenancypg.NewMembershipRepository(pool)
	rootMembershipRepo := tenancypg.NewRootMembershipRepository(root)

	ownerID := insertUser(t, root)
	strangerID := insertUser(t, root)
	org, _ := domain.NewOrganization("Root Membership Isolation Org", "root-membership-iso-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, ownerID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	if err := membershipRepo.Create(ctx, domain.NewOrganizationMembership(org.ID, ownerID)); err != nil {
		t.Fatalf("Create membership: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organization_memberships WHERE organization_id = $1`, org.ID)
		dbtest.DeleteOrganization(t, root, org.ID)
	})

	strangerOrgs, err := rootMembershipRepo.ListOrganizationsForUser(ctx, strangerID)
	if err != nil {
		t.Fatalf("ListOrganizationsForUser (stranger): %v", err)
	}
	if len(strangerOrgs) != 0 {
		t.Fatalf("expected a user with no membership in this org to see zero results, got %d", len(strangerOrgs))
	}

	ownerOrgs, err := rootMembershipRepo.ListOrganizationsForUser(ctx, ownerID)
	if err != nil {
		t.Fatalf("ListOrganizationsForUser (owner): %v", err)
	}
	if len(ownerOrgs) != 1 || ownerOrgs[0].ID != org.ID {
		t.Fatalf("expected the real owner to see exactly their own org, got %+v", ownerOrgs)
	}
}

func TestRootMembershipRepository_IncludesArchivedOrganizations(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	membershipRepo := tenancypg.NewMembershipRepository(pool)
	rootMembershipRepo := tenancypg.NewRootMembershipRepository(root)

	actorID := insertUser(t, root)
	org, _ := domain.NewOrganization("Root Membership Archived Org", "root-membership-archived-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	if err := membershipRepo.Create(ctx, domain.NewOrganizationMembership(org.ID, actorID)); err != nil {
		t.Fatalf("Create membership: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organization_memberships WHERE organization_id = $1`, org.ID)
		dbtest.DeleteOrganization(t, root, org.ID)
	})

	if err := org.Archive(); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := orgRepo.Archive(ctx, org, actorID); err != nil {
		t.Fatalf("orgRepo.Archive: %v", err)
	}

	orgs, err := rootMembershipRepo.ListOrganizationsForUser(ctx, actorID)
	if err != nil {
		t.Fatalf("ListOrganizationsForUser: %v", err)
	}
	found := false
	for _, o := range orgs {
		if o.ID == org.ID {
			found = true
			if o.Status != "archived" {
				t.Errorf("expected the returned org's own status to reflect archived, got %q", o.Status)
			}
		}
	}
	if !found {
		t.Error("expected an archived organization to still appear in the caller's own list, not be silently filtered")
	}
}
