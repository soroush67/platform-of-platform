package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	tenancypg "platform-of-platform/internal/tenancy/adapters/postgres"
	"platform-of-platform/internal/tenancy/domain"
)

func TestMembershipRepository_CreateAndIsMember(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	membershipRepo := tenancypg.NewMembershipRepository(pool)

	// Both insertUser calls deliberately come before the org (and its
	// cleanup) is created - t.Cleanup runs LIFO, and the organization's
	// own cleanup below deletes organization_memberships (which
	// forward-references these users); registering it *after* both
	// insertUser calls means it runs *before* their own user-row deletes,
	// avoiding the FK violation a wrong order would hit (found for real:
	// the first version of this test registered memberID's insertUser
	// after the org's cleanup and failed with exactly that violation).
	actorID := insertUser(t, root)
	memberID := insertUser(t, root)
	org, _ := domain.NewOrganization("Membership Test Org", "membership-org-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organization_memberships WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, org.ID)
	})

	isMember, err := membershipRepo.IsMember(ctx, org.ID, memberID)
	if err != nil {
		t.Fatalf("IsMember (before): %v", err)
	}
	if isMember {
		t.Fatal("expected a user with no membership row to not be a member")
	}

	m := domain.NewOrganizationMembership(org.ID, memberID)
	if err := membershipRepo.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	isMember, err = membershipRepo.IsMember(ctx, org.ID, memberID)
	if err != nil {
		t.Fatalf("IsMember (after): %v", err)
	}
	if !isMember {
		t.Error("expected the user to be a member after Create")
	}

	// Cross-org: same user, wrong org - RLS/the query's own WHERE must
	// both agree this is false.
	isMember, err = membershipRepo.IsMember(ctx, uuid.NewString(), memberID)
	if err != nil {
		t.Fatalf("IsMember (wrong org): %v", err)
	}
	if isMember {
		t.Error("expected a real member to not be a member of an unrelated organization")
	}
}

// TestMembershipRepository_IsMember_RecognizesServiceAccounts is the
// regression test for IsMember's own doc comment: a ServiceAccount has
// no organization_memberships row (it's scoped directly by its own
// organization_id column), so IsMember has a second EXISTS clause
// specifically for that - this proves that clause actually works
// against a real service_accounts row, not just that the SQL parses.
func TestMembershipRepository_IsMember_RecognizesServiceAccounts(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	membershipRepo := tenancypg.NewMembershipRepository(pool)

	actorID := insertUser(t, root)
	org, _ := domain.NewOrganization("SA Membership Test Org", "sa-membership-org-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM audit_entries WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM service_accounts WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM organizations WHERE id = $1`, org.ID)
	})

	saID := uuid.NewString()
	mustExec(t, root, `INSERT INTO service_accounts (id, organization_id, name) VALUES ($1, $2, 'test-sa')`, saID, org.ID)

	isMember, err := membershipRepo.IsMember(ctx, org.ID, saID)
	if err != nil {
		t.Fatalf("IsMember: %v", err)
	}
	if !isMember {
		t.Error("expected a real ServiceAccount belonging to this org to count as a member")
	}
}
