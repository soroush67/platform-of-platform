package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func setupOrgWithMember(t *testing.T, orgRepo *fakeOrgRepo, membershipRepo *fakeMembershipRepo, userID string) *domain.Organization {
	t.Helper()
	org, err := domain.NewOrganization("Acme", "acme")
	if err != nil {
		t.Fatalf("NewOrganization: %v", err)
	}
	orgRepo.put(org)
	membershipRepo.add(org.ID, userID)
	return org
}

func TestCreateProjectService_RequiresProjectManage(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	permChecker := newFakePermChecker()
	projectRepo := newFakeProjectRepo()
	svc := application.NewCreateProjectService(projectRepo, membershipRepo, permChecker, orgRepo)

	_, err := svc.Execute(context.Background(), application.CreateProjectInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", Name: "p", Slug: "p",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without project:manage, got: %v", err)
	}

	permChecker.grant(org.ID, "member-1", "project:manage")
	project, err := svc.Execute(context.Background(), application.CreateProjectInput{
		OrganizationID: org.ID, RequestingUserID: "member-1", Name: "p", Slug: "p",
	})
	if err != nil {
		t.Fatalf("expected creation to succeed once granted, got: %v", err)
	}
	if project.OrganizationID != org.ID {
		t.Errorf("expected project scoped to org %q, got %q", org.ID, project.OrganizationID)
	}
}

func TestCreateProjectService_ArchivedOrgRejected(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "owner-1")
	_ = org.Archive()
	orgRepo.put(org)
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "owner-1", "project:manage")
	svc := application.NewCreateProjectService(newFakeProjectRepo(), membershipRepo, permChecker, orgRepo)

	_, err := svc.Execute(context.Background(), application.CreateProjectInput{
		OrganizationID: org.ID, RequestingUserID: "owner-1", Name: "p", Slug: "p",
	})
	if !errors.Is(err, domain.ErrOrganizationArchived) {
		t.Fatalf("expected ErrOrganizationArchived, got: %v", err)
	}
}

func TestCreateProjectService_NonMemberGetsOrgNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "real-member")
	svc := application.NewCreateProjectService(newFakeProjectRepo(), membershipRepo, newFakePermChecker(), orgRepo)

	_, err := svc.Execute(context.Background(), application.CreateProjectInput{
		OrganizationID: org.ID, RequestingUserID: "stranger", Name: "p", Slug: "p",
	})
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for a non-member (not ErrForbidden - don't reveal existence), got: %v", err)
	}
}

func TestDeleteProjectService_RequiresProjectDelete(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "member-1")
	projectRepo := newFakeProjectRepo()
	project, _ := domain.NewProject(org.ID, "p", "p", "")
	_ = projectRepo.Create(context.Background(), project)

	permChecker := newFakePermChecker()
	svc := application.NewDeleteProjectService(projectRepo, projectRepo, membershipRepo, permChecker)

	err := svc.Execute(context.Background(), application.DeleteProjectInput{
		OrganizationID: org.ID, ProjectID: project.ID, RequestingUserID: "member-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without project:delete, got: %v", err)
	}

	permChecker.grant(org.ID, "member-1", "project:delete")
	if err := svc.Execute(context.Background(), application.DeleteProjectInput{
		OrganizationID: org.ID, ProjectID: project.ID, RequestingUserID: "member-1",
	}); err != nil {
		t.Fatalf("expected deletion to succeed once granted, got: %v", err)
	}
	if _, err := projectRepo.GetByID(context.Background(), org.ID, project.ID); !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected project to be gone after Purge, got: %v", err)
	}
}

func TestDeleteProjectService_NonMemberGetsOrgNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "real-member")
	projectRepo := newFakeProjectRepo()
	project, _ := domain.NewProject(org.ID, "p", "p", "")
	_ = projectRepo.Create(context.Background(), project)
	svc := application.NewDeleteProjectService(projectRepo, projectRepo, membershipRepo, newFakePermChecker())

	err := svc.Execute(context.Background(), application.DeleteProjectInput{
		OrganizationID: org.ID, ProjectID: project.ID, RequestingUserID: "stranger",
	})
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for a non-member (not ErrForbidden - don't reveal existence), got: %v", err)
	}
}

func TestDeleteProjectService_UnknownProjectNotFound(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin")
	projectRepo := newFakeProjectRepo()
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin", "project:delete")
	svc := application.NewDeleteProjectService(projectRepo, projectRepo, membershipRepo, permChecker)

	err := svc.Execute(context.Background(), application.DeleteProjectInput{
		OrganizationID: org.ID, ProjectID: "nonexistent", RequestingUserID: "admin",
	})
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got: %v", err)
	}
}

func TestGetProjectService_OwnerAdminBypassesVisibility(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin")
	projectRepo := newFakeProjectRepo()
	project, _ := domain.NewProject(org.ID, "p", "p", "")
	_ = projectRepo.Create(context.Background(), project)

	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin", "organization:manage")
	svc := application.NewGetProjectService(projectRepo, membershipRepo, permChecker, newFakeVisibilityChecker())

	got, err := svc.Execute(context.Background(), org.ID, project.ID, "admin")
	if err != nil {
		t.Fatalf("expected organization:manage to bypass per-project visibility entirely, got: %v", err)
	}
	if got.ID != project.ID {
		t.Errorf("expected project %q, got %q", project.ID, got.ID)
	}
}

func TestGetProjectService_RequiresProjectScopeGrant(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "reader")
	projectRepo := newFakeProjectRepo()
	project, _ := domain.NewProject(org.ID, "p", "p", "")
	_ = projectRepo.Create(context.Background(), project)

	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(org.ID, "reader", "project:read", "project", project.ID)
	svc := application.NewGetProjectService(projectRepo, membershipRepo, newFakePermChecker(), visibilityChecker)

	got, err := svc.Execute(context.Background(), org.ID, project.ID, "reader")
	if err != nil {
		t.Fatalf("expected a project-scope project:read grant to allow reading it, got: %v", err)
	}
	if got.ID != project.ID {
		t.Errorf("expected project %q, got %q", project.ID, got.ID)
	}

	// An org-wide member with no project-scope grant at all (and not
	// organization:manage) is the whole point of this change - a plain
	// org member no longer sees every project by default.
	svc = application.NewGetProjectService(projectRepo, membershipRepo, newFakePermChecker(), newFakeVisibilityChecker())
	if _, err := svc.Execute(context.Background(), org.ID, project.ID, "reader"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without a project-scope grant or organization:manage, got: %v", err)
	}
}

func TestListProjectsService_ScopedToOrganization(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	orgA := setupOrgWithMember(t, orgRepo, membershipRepo, "user-1")
	orgB, _ := domain.NewOrganization("Other", "other")
	orgRepo.put(orgB)
	membershipRepo.add(orgB.ID, "user-1")

	projectRepo := newFakeProjectRepo()
	pA, _ := domain.NewProject(orgA.ID, "a", "a", "")
	pB, _ := domain.NewProject(orgB.ID, "b", "b", "")
	_ = projectRepo.Create(context.Background(), pA)
	_ = projectRepo.Create(context.Background(), pB)

	visibilityChecker := newFakeVisibilityChecker()
	visibilityChecker.grant(orgA.ID, "user-1", "project:read", "project", pA.ID)
	svc := application.NewListProjectsService(projectRepo, membershipRepo, newFakePermChecker(), visibilityChecker)
	got, err := svc.Execute(context.Background(), orgA.ID, "user-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != pA.ID {
		t.Errorf("expected exactly org A's own project, got %+v", got)
	}
}

func TestListProjectsService_HiddenWithoutGrant(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "user-1")
	projectRepo := newFakeProjectRepo()
	project, _ := domain.NewProject(org.ID, "p", "p", "")
	_ = projectRepo.Create(context.Background(), project)

	// A plain member with zero project-scope grants sees an empty list -
	// a valid, non-error outcome, not ErrForbidden (they're a real
	// member, they just can't see any Project yet).
	svc := application.NewListProjectsService(projectRepo, membershipRepo, newFakePermChecker(), newFakeVisibilityChecker())
	got, err := svc.Execute(context.Background(), org.ID, "user-1")
	if err != nil {
		t.Fatalf("expected no error, just an empty list, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero visible projects, got %+v", got)
	}
}

func TestListProjectsService_OwnerAdminSeesEverything(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "admin")
	projectRepo := newFakeProjectRepo()
	pA, _ := domain.NewProject(org.ID, "a", "a", "")
	pB, _ := domain.NewProject(org.ID, "b", "b", "")
	_ = projectRepo.Create(context.Background(), pA)
	_ = projectRepo.Create(context.Background(), pB)

	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "admin", "organization:manage")
	svc := application.NewListProjectsService(projectRepo, membershipRepo, permChecker, newFakeVisibilityChecker())
	got, err := svc.Execute(context.Background(), org.ID, "admin")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected organization:manage to see every project regardless of any per-project grant, got %+v", got)
	}
}
