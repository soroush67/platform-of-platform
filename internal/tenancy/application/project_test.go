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

func TestCreateProjectService_RequiresOrganizationManage(t *testing.T) {
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
		t.Fatalf("expected ErrForbidden without organization:manage, got: %v", err)
	}

	permChecker.grant(org.ID, "member-1", "organization:manage")
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
	permChecker.grant(org.ID, "owner-1", "organization:manage")
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

func TestGetProjectService_ReadOnlyMembershipGate(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	org := setupOrgWithMember(t, orgRepo, membershipRepo, "reader")
	projectRepo := newFakeProjectRepo()
	project, _ := domain.NewProject(org.ID, "p", "p", "")
	_ = projectRepo.Create(context.Background(), project)

	svc := application.NewGetProjectService(projectRepo, membershipRepo)

	// Any member, no permission grant needed - reads are membership-gated
	// only in this codebase.
	got, err := svc.Execute(context.Background(), org.ID, project.ID, "reader")
	if err != nil {
		t.Fatalf("expected a member to read the project without any RBAC grant, got: %v", err)
	}
	if got.ID != project.ID {
		t.Errorf("expected project %q, got %q", project.ID, got.ID)
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

	svc := application.NewListProjectsService(projectRepo, membershipRepo)
	got, err := svc.Execute(context.Background(), orgA.ID, "user-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 || got[0].ID != pA.ID {
		t.Errorf("expected exactly org A's own project, got %+v", got)
	}
}
