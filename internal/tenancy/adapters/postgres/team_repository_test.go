package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"platform-of-platform/internal/platform/dbtest"
	tenancypg "platform-of-platform/internal/tenancy/adapters/postgres"
	"platform-of-platform/internal/tenancy/domain"
)

func setupTeamTest(t *testing.T) (*tenancypg.TeamRepository, *domain.Organization, string) {
	t.Helper()
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	teamRepo := tenancypg.NewTeamRepository(pool)

	actorID := insertUser(t, root)
	org, _ := domain.NewOrganization("Team Test Org", "team-test-org-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM team_memberships WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM teams WHERE organization_id = $1`, org.ID)
		dbtest.DeleteOrganization(t, root, org.ID)
	})

	return teamRepo, org, actorID
}

func TestTeamRepository_CreateAndGetByID(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()

	team, err := domain.NewTeam(org.ID, "Platform Team")
	if err != nil {
		t.Fatalf("NewTeam: %v", err)
	}
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := teamRepo.GetByID(ctx, org.ID, team.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Platform Team" {
		t.Errorf("expected name %q, got %q", "Platform Team", got.Name)
	}
}

func TestTeamRepository_GetByID_WrongOrganizationReturnsNotFound(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()

	team, _ := domain.NewTeam(org.ID, "Platform Team")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := teamRepo.GetByID(ctx, uuid.NewString(), team.ID)
	if !errors.Is(err, domain.ErrTeamNotFound) {
		t.Fatalf("expected ErrTeamNotFound for a team under a different org, got: %v", err)
	}
}

func TestTeamRepository_TeamExists(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()

	exists, err := teamRepo.TeamExists(ctx, org.ID, uuid.NewString())
	if err != nil {
		t.Fatalf("TeamExists (unknown): %v", err)
	}
	if exists {
		t.Error("expected an unknown team id to not exist")
	}

	team, _ := domain.NewTeam(org.ID, "Platform Team")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create: %v", err)
	}

	exists, err = teamRepo.TeamExists(ctx, org.ID, team.ID)
	if err != nil {
		t.Fatalf("TeamExists (real): %v", err)
	}
	if !exists {
		t.Error("expected a real team to exist")
	}
}

func TestTeamRepository_AddMemberIsIdempotent(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()
	root := dbtest.RootPool(t)

	team, _ := domain.NewTeam(org.ID, "Platform Team")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	userID := insertUser(t, root)
	// Registered after insertUser's own cleanup (t.Cleanup runs LIFO),
	// so this row - which forward-references userID - gets deleted
	// before insertUser's own DELETE FROM users runs, avoiding the FK
	// violation the wrong order hits (same class of ordering bug fixed
	// in membership_repository_test.go).
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM team_memberships WHERE team_id = $1 AND user_id = $2`, team.ID, userID)
	})

	m := domain.NewTeamMembership(team.ID, org.ID, userID)
	if err := teamRepo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember (first): %v", err)
	}
	// AddMember's own ON CONFLICT (team_id, user_id) DO NOTHING - a
	// second add for the same pair must succeed as a silent no-op, not
	// a unique-constraint error.
	m2 := domain.NewTeamMembership(team.ID, org.ID, userID)
	if err := teamRepo.AddMember(ctx, m2); err != nil {
		t.Fatalf("AddMember (duplicate): %v", err)
	}

	var count int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM team_memberships WHERE team_id = $1 AND user_id = $2`, team.ID, userID).Scan(&count); err != nil {
		t.Fatalf("query team_memberships: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 membership row despite two AddMember calls, got %d", count)
	}
}

func TestTeamRepository_RemoveMember(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()
	root := dbtest.RootPool(t)

	team, _ := domain.NewTeam(org.ID, "Platform Team")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	userID := insertUser(t, root)
	m := domain.NewTeamMembership(team.ID, org.ID, userID)
	if err := teamRepo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	if err := teamRepo.RemoveMember(ctx, org.ID, team.ID, userID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	var count int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM team_memberships WHERE team_id = $1 AND user_id = $2`, team.ID, userID).Scan(&count); err != nil {
		t.Fatalf("query team_memberships: %v", err)
	}
	if count != 0 {
		t.Errorf("expected the membership row to be gone after RemoveMember, got count=%d", count)
	}
}
