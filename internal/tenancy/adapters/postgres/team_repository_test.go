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

func TestTeamRepository_Create_DuplicateNameRejected(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()

	team1, _ := domain.NewTeam(org.ID, "Platform Team")
	if err := teamRepo.Create(ctx, team1); err != nil {
		t.Fatalf("Create (first): %v", err)
	}

	team2, _ := domain.NewTeam(org.ID, "Platform Team")
	err := teamRepo.Create(ctx, team2)
	if !errors.Is(err, domain.ErrTeamAlreadyExists) {
		t.Fatalf("expected ErrTeamAlreadyExists for a duplicate name in the same org, got: %v", err)
	}
}

func TestTeamRepository_ListMembers(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()
	root := dbtest.RootPool(t)

	team, _ := domain.NewTeam(org.ID, "Platform Team")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	otherTeam, _ := domain.NewTeam(org.ID, "Other Team")
	if err := teamRepo.Create(ctx, otherTeam); err != nil {
		t.Fatalf("Create other team: %v", err)
	}

	userID := insertUser(t, root)
	otherUserID := insertUser(t, root)
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM team_memberships WHERE team_id IN ($1, $2)`, team.ID, otherTeam.ID)
	})

	if err := teamRepo.AddMember(ctx, domain.NewTeamMembership(team.ID, org.ID, userID)); err != nil {
		t.Fatalf("AddMember (team, userID): %v", err)
	}
	// A membership on a DIFFERENT team must not leak into this team's
	// own roster - the real point of this test (the query's own WHERE
	// team_id = $1, not just "any membership in this org").
	if err := teamRepo.AddMember(ctx, domain.NewTeamMembership(otherTeam.ID, org.ID, otherUserID)); err != nil {
		t.Fatalf("AddMember (otherTeam, otherUserID): %v", err)
	}

	members, err := teamRepo.ListMembers(ctx, org.ID, team.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 || members[0].UserID != userID {
		t.Errorf("expected exactly the one member of this team, got %+v", members)
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

func TestTeamRepository_Update_RenamesInPlace(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()

	team, _ := domain.NewTeam(org.ID, "platfrom-mamanger")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create: %v", err)
	}

	team.Name = "platform-manager"
	if err := teamRepo.Update(ctx, team); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := teamRepo.GetByID(ctx, org.ID, team.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "platform-manager" {
		t.Errorf("expected the renamed name, got %q", got.Name)
	}
}

func TestTeamRepository_Update_DuplicateNameRejected(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()

	team1, _ := domain.NewTeam(org.ID, "team-one")
	if err := teamRepo.Create(ctx, team1); err != nil {
		t.Fatalf("Create team1: %v", err)
	}
	team2, _ := domain.NewTeam(org.ID, "team-two")
	if err := teamRepo.Create(ctx, team2); err != nil {
		t.Fatalf("Create team2: %v", err)
	}

	team2.Name = "team-one"
	err := teamRepo.Update(ctx, team2)
	if !errors.Is(err, domain.ErrTeamAlreadyExists) {
		t.Fatalf("expected ErrTeamAlreadyExists renaming to an already-taken name, got: %v", err)
	}
}

// TestTeamRepository_Delete_RemovesTeamAndItsOwnMemberships is the real
// regression test for the FK problem this session found: deleting a
// teams row before its team_memberships would hit a real constraint
// violation (team_memberships.team_id has no ON DELETE CASCADE).
func TestTeamRepository_Delete_RemovesTeamAndItsOwnMemberships(t *testing.T) {
	teamRepo, org, _ := setupTeamTest(t)
	ctx := context.Background()
	root := dbtest.RootPool(t)

	team, _ := domain.NewTeam(org.ID, "delete-me-team")
	if err := teamRepo.Create(ctx, team); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	userID := insertUser(t, root)
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM team_memberships WHERE team_id = $1`, team.ID)
	})
	if err := teamRepo.AddMember(ctx, domain.NewTeamMembership(team.ID, org.ID, userID)); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	if err := teamRepo.Delete(ctx, org.ID, team.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := teamRepo.GetByID(ctx, org.ID, team.ID); !errors.Is(err, domain.ErrTeamNotFound) {
		t.Errorf("expected the team to be gone, got: %v", err)
	}

	var membershipCount int
	if err := root.QueryRow(ctx, `SELECT count(*) FROM team_memberships WHERE team_id = $1`, team.ID).Scan(&membershipCount); err != nil {
		t.Fatalf("query team_memberships: %v", err)
	}
	if membershipCount != 0 {
		t.Errorf("expected the team's own memberships to be gone too, got %d", membershipCount)
	}
}
