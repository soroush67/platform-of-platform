package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

func mustOrg(t *testing.T, name, slug string) *domain.Organization {
	t.Helper()
	org, err := domain.NewOrganization(name, slug)
	if err != nil {
		t.Fatalf("NewOrganization: %v", err)
	}
	return org
}

func TestListMyOrganizationsService_ReturnsEveryOrgForTheCaller(t *testing.T) {
	repo := newFakeRootMembershipRepo()
	orgA := mustOrg(t, "Org A", "org-a")
	orgB := mustOrg(t, "Org B", "org-b")
	repo.addMembership("user-1", orgA)
	repo.addMembership("user-1", orgB)
	svc := application.NewListMyOrganizationsService(repo)

	orgs, err := svc.Execute(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("expected 2 orgs, got %d", len(orgs))
	}
}

func TestListMyOrganizationsService_NoMembershipsReturnsEmptyNotError(t *testing.T) {
	svc := application.NewListMyOrganizationsService(newFakeRootMembershipRepo())

	orgs, err := svc.Execute(context.Background(), "user-with-no-orgs")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(orgs) != 0 {
		t.Errorf("expected zero orgs, got %d", len(orgs))
	}
}

func TestListMyOrganizationsService_PropagatesRepoError(t *testing.T) {
	repo := newFakeRootMembershipRepo()
	repo.err = errors.New("boom")
	svc := application.NewListMyOrganizationsService(repo)

	_, err := svc.Execute(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected the repo error to propagate")
	}
}
