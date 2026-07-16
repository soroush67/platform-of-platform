package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

// newCreateOrganizationService wires a CreateOrganizationService with
// real, independently-controllable fakes for the platform-admin gate -
// callers configure rootMembership's org count and platformAdmin's
// admin set per test.
func newCreateOrganizationService(orgRepo *fakeOrgRepo, membershipRepo *fakeMembershipRepo, roleAssigner *fakeRoleAssigner) (*application.CreateOrganizationService, *fakeRootMembershipRepo, *fakePlatformAdmin) {
	rootMembership := newFakeRootMembershipRepo()
	platformAdmin := newFakePlatformAdmin()
	svc := application.NewCreateOrganizationService(orgRepo, membershipRepo, roleAssigner, rootMembership, platformAdmin, platformAdmin)
	return svc, rootMembership, platformAdmin
}

func TestCreateOrganizationService_FirstOrgEverBootstrapsCreatorAsPlatformAdmin(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	membershipRepo := newFakeMembershipRepo()
	roleAssigner := newFakeRoleAssigner()
	svc, rootMembership, platformAdmin := newCreateOrganizationService(orgRepo, membershipRepo, roleAssigner)
	rootMembership.setOrgCount(0) // no Organization exists anywhere yet

	org, err := svc.Execute(context.Background(), application.CreateOrganizationInput{
		Name: "Acme", Slug: "acme", CreatedByUserID: "user-1",
	})
	if err != nil {
		t.Fatalf("expected the platform's first-ever org creation to succeed without any prior admin grant, got: %v", err)
	}

	if _, err := orgRepo.GetByID(context.Background(), org.ID); err != nil {
		t.Errorf("expected org to be persisted, got: %v", err)
	}
	isMember, _ := membershipRepo.IsMember(context.Background(), org.ID, "user-1")
	if !isMember {
		t.Error("expected the creator to be a member of the new org")
	}
	if got := roleAssigner.roleOf(org.ID, "user-1"); got != "owner" {
		t.Errorf("expected creator to be assigned 'owner', got %q", got)
	}
	isAdmin, _ := platformAdmin.IsPlatformAdmin(context.Background(), "user-1")
	if !isAdmin {
		t.Error("expected the first-ever org's creator to be granted is_platform_admin as a side effect")
	}
}

func TestCreateOrganizationService_NonAdminForbiddenOnceAnOrgAlreadyExists(t *testing.T) {
	svc, rootMembership, _ := newCreateOrganizationService(newFakeOrgRepo(), newFakeMembershipRepo(), newFakeRoleAssigner())
	rootMembership.setOrgCount(1) // the platform's bootstrap window has already closed

	_, err := svc.Execute(context.Background(), application.CreateOrganizationInput{
		Name: "Acme", Slug: "acme", CreatedByUserID: "user-1",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-platform-admin once at least one org exists, got: %v", err)
	}
}

func TestCreateOrganizationService_ExistingPlatformAdminCanCreateAnotherOrg(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	svc, rootMembership, platformAdmin := newCreateOrganizationService(orgRepo, newFakeMembershipRepo(), newFakeRoleAssigner())
	rootMembership.setOrgCount(1)
	if err := platformAdmin.SetPlatformAdmin(context.Background(), "admin-1", true); err != nil {
		t.Fatalf("SetPlatformAdmin: %v", err)
	}

	org, err := svc.Execute(context.Background(), application.CreateOrganizationInput{
		Name: "Second Org", Slug: "second-org", CreatedByUserID: "admin-1",
	})
	if err != nil {
		t.Fatalf("expected an existing platform admin to create a second org, got: %v", err)
	}
	if _, err := orgRepo.GetByID(context.Background(), org.ID); err != nil {
		t.Errorf("expected org to be persisted, got: %v", err)
	}
}

func TestCreateOrganizationService_InvalidSlugRejectedBeforeAnyWrite(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	svc, rootMembership, _ := newCreateOrganizationService(orgRepo, newFakeMembershipRepo(), newFakeRoleAssigner())
	rootMembership.setOrgCount(0) // bootstrap window - passes the admin gate so slug validation is what's actually being tested

	_, err := svc.Execute(context.Background(), application.CreateOrganizationInput{
		Name: "Acme", Slug: "Not A Valid Slug!", CreatedByUserID: "user-1",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an invalid slug, got: %v", err)
	}
}

func TestGetOrganizationService_NonMemberGetsNotFoundNotForbidden(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	svc := application.NewGetOrganizationService(orgRepo, membershipRepo)

	// A non-member requesting a real org gets ErrOrganizationNotFound,
	// not a distinguishable "forbidden" - the exact "don't reveal
	// existence" invariant this service's own doc comment names.
	_, err := svc.Execute(context.Background(), org.ID, "stranger")
	if !errors.Is(err, domain.ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound for a non-member, got: %v", err)
	}

	membershipRepo.add(org.ID, "member-1")
	got, err := svc.Execute(context.Background(), org.ID, "member-1")
	if err != nil {
		t.Fatalf("expected a member to read the org, got: %v", err)
	}
	if got.ID != org.ID {
		t.Errorf("expected org %q, got %q", org.ID, got.ID)
	}
}

func TestArchiveOrganizationService_RequiresOrganizationDeletePermission(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add(org.ID, "admin-user")
	permChecker := newFakePermChecker()
	svc := application.NewArchiveOrganizationService(orgRepo, orgRepo, membershipRepo, permChecker)

	// Admin, without organization:delete (the real Owner/Admin
	// divergence this permission exists for) - must be denied.
	_, err := svc.Execute(context.Background(), application.ArchiveOrganizationInput{
		OrganizationID: org.ID, RequestingUserID: "admin-user",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without organization:delete, got: %v", err)
	}

	permChecker.grant(org.ID, "admin-user", "organization:delete")
	archived, err := svc.Execute(context.Background(), application.ArchiveOrganizationInput{
		OrganizationID: org.ID, RequestingUserID: "admin-user",
	})
	if err != nil {
		t.Fatalf("expected archive to succeed once granted, got: %v", err)
	}
	if archived.Status != domain.OrganizationStatusArchived {
		t.Errorf("expected status archived, got %q", archived.Status)
	}
	if archived.ArchivedAt == nil {
		t.Error("expected ArchivedAt to be set")
	}
}

func TestArchiveOrganizationService_AlreadyArchivedRejected(t *testing.T) {
	orgRepo := newFakeOrgRepo()
	org, _ := domain.NewOrganization("Acme", "acme")
	_ = org.Archive()
	orgRepo.put(org)
	membershipRepo := newFakeMembershipRepo()
	membershipRepo.add(org.ID, "owner-user")
	permChecker := newFakePermChecker()
	permChecker.grant(org.ID, "owner-user", "organization:delete")
	svc := application.NewArchiveOrganizationService(orgRepo, orgRepo, membershipRepo, permChecker)

	_, err := svc.Execute(context.Background(), application.ArchiveOrganizationInput{
		OrganizationID: org.ID, RequestingUserID: "owner-user",
	})
	if !errors.Is(err, domain.ErrOrganizationAlreadyArchived) {
		t.Fatalf("expected ErrOrganizationAlreadyArchived, got: %v", err)
	}
}
