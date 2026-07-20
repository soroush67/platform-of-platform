package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

func TestArchiveMachineService_NeverAttemptsDelete(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1"}
	svc := application.NewArchiveMachineService(repo, membership, permChecker)

	if err := svc.Execute(context.Background(), "org-1", "user-1", "m-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !repo.archived["m-1"] {
		t.Errorf("expected Archive to be called")
	}
	if _, ok := repo.machines["m-1"]; !ok {
		t.Errorf("expected the machine row to still exist - Archive must never hard-delete")
	}
}

func TestArchiveMachineService_RequiresMachineManage(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1"}
	svc := application.NewArchiveMachineService(repo, membership, newFakeAttachmentPermChecker())

	err := svc.Execute(context.Background(), "org-1", "user-1", "m-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without machine:manage, got: %v", err)
	}
}

func TestDeleteMachineService_HardDeletesWhenNoHistory(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:delete")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1"}
	svc := application.NewDeleteMachineService(repo, membership, permChecker)

	if err := svc.Execute(context.Background(), "org-1", "user-1", "m-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := repo.machines["m-1"]; ok {
		t.Errorf("expected the machine row to be gone after a real hard delete")
	}
	if repo.archived["m-1"] {
		t.Errorf("expected no archive fallback - DeleteMachineService must never silently archive")
	}
}

func TestDeleteMachineService_RealConflictOnHistoryNoFallback(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:delete")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1"}
	repo.hasHistory["m-1"] = true
	svc := application.NewDeleteMachineService(repo, membership, permChecker)

	err := svc.Execute(context.Background(), "org-1", "user-1", "m-1")
	if !errors.Is(err, domain.ErrMachineHasHistory) {
		t.Fatalf("expected ErrMachineHasHistory to propagate as a real error, got: %v", err)
	}
	if _, ok := repo.machines["m-1"]; !ok {
		t.Errorf("expected the machine row to still exist - a blocked delete must not remove it")
	}
	if repo.archived["m-1"] {
		t.Errorf("expected no silent archive fallback")
	}
}

func TestDeleteMachineService_RequiresMachineDelete(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	// Granting only machine:manage (not machine:delete) must not be enough -
	// the whole point of the new permission is that manage no longer
	// implies delete.
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1"}
	svc := application.NewDeleteMachineService(repo, membership, permChecker)

	err := svc.Execute(context.Background(), "org-1", "user-1", "m-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden with only machine:manage (not machine:delete), got: %v", err)
	}
}
