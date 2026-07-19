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

func setupProjectTest(t *testing.T) (*tenancypg.OrganizationRepository, *tenancypg.ProjectRepository, *domain.Organization) {
	t.Helper()
	ctx := context.Background()
	pool := dbtest.AppPool(t)
	root := dbtest.RootPool(t)
	orgRepo := tenancypg.NewOrganizationRepository(pool)
	projectRepo := tenancypg.NewProjectRepository(pool)
	actorID := insertUser(t, root)

	org, _ := domain.NewOrganization("Project Test Org", "project-test-org-"+uuid.NewString()[:8])
	if err := orgRepo.Create(ctx, org, actorID); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, root, `DELETE FROM outbox_events WHERE organization_id = $1`, org.ID)
		mustExec(t, root, `DELETE FROM projects WHERE organization_id = $1`, org.ID)
		dbtest.DeleteOrganization(t, root, org.ID)
	})

	return orgRepo, projectRepo, org
}

func TestProjectRepository_CreateAndGetByID(t *testing.T) {
	_, projectRepo, org := setupProjectTest(t)
	ctx := context.Background()

	project, err := domain.NewProject(org.ID, "Test Project", "test-project", "a description")
	if err != nil {
		t.Fatalf("NewProject: %v", err)
	}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := projectRepo.GetByID(ctx, org.ID, project.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Test Project" || got.Slug != "test-project" || got.Description != "a description" {
		t.Errorf("expected fields to round-trip, got %+v", got)
	}
}

func TestProjectRepository_GetByID_WrongOrganizationReturnsNotFound(t *testing.T) {
	_, projectRepo, org := setupProjectTest(t)
	ctx := context.Background()

	project, _ := domain.NewProject(org.ID, "Test Project", "test-project", "")
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Same belt-and-braces property GetByID's own doc comment claims:
	// asking for a real project id under the WRONG organizationID must
	// come back not-found, not the project - RLS plus the explicit WHERE
	// clause both have to agree for this to hold.
	_, err := projectRepo.GetByID(ctx, uuid.NewString(), project.ID)
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound for a project under a different org, got: %v", err)
	}
}

func TestProjectRepository_ProjectExists(t *testing.T) {
	_, projectRepo, org := setupProjectTest(t)
	ctx := context.Background()

	exists, err := projectRepo.ProjectExists(ctx, org.ID, uuid.NewString())
	if err != nil {
		t.Fatalf("ProjectExists (unknown): %v", err)
	}
	if exists {
		t.Error("expected an unknown project id to not exist")
	}

	project, _ := domain.NewProject(org.ID, "Test Project", "test-project", "")
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Create: %v", err)
	}

	exists, err = projectRepo.ProjectExists(ctx, org.ID, project.ID)
	if err != nil {
		t.Fatalf("ProjectExists (real): %v", err)
	}
	if !exists {
		t.Error("expected a real project to exist")
	}

	// Cross-org: same id, wrong org - RLS must hide it.
	exists, err = projectRepo.ProjectExists(ctx, uuid.NewString(), project.ID)
	if err != nil {
		t.Fatalf("ProjectExists (wrong org): %v", err)
	}
	if exists {
		t.Error("expected a real project id under the wrong organization_id to not exist")
	}
}

func TestProjectRepository_ListByOrganization(t *testing.T) {
	_, projectRepo, org := setupProjectTest(t)
	ctx := context.Background()

	p1, _ := domain.NewProject(org.ID, "Project One", "project-one", "")
	p2, _ := domain.NewProject(org.ID, "Project Two", "project-two", "")
	if err := projectRepo.Create(ctx, p1); err != nil {
		t.Fatalf("Create p1: %v", err)
	}
	if err := projectRepo.Create(ctx, p2); err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	got, err := projectRepo.ListByOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListByOrganization: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}

	// A second, unrelated org must never see these - the actual RLS
	// property this list query depends on.
	otherOrgProjects, err := projectRepo.ListByOrganization(ctx, uuid.NewString())
	if err != nil {
		t.Fatalf("ListByOrganization (other org): %v", err)
	}
	if len(otherOrgProjects) != 0 {
		t.Errorf("expected zero projects for an unrelated organization, got %d", len(otherOrgProjects))
	}
}

func TestProjectRepository_Create_DuplicateSlugRejected(t *testing.T) {
	_, projectRepo, org := setupProjectTest(t)
	ctx := context.Background()

	project1, _ := domain.NewProject(org.ID, "Project One", "dup-slug", "")
	if err := projectRepo.Create(ctx, project1); err != nil {
		t.Fatalf("Create (first): %v", err)
	}

	project2, _ := domain.NewProject(org.ID, "Project Two", "dup-slug", "")
	err := projectRepo.Create(ctx, project2)
	if !errors.Is(err, domain.ErrProjectAlreadyExists) {
		t.Fatalf("expected ErrProjectAlreadyExists for a duplicate slug in the same org, got: %v", err)
	}
}
