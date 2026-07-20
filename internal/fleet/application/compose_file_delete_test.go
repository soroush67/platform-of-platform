package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

func TestDeleteComposeFileService_HardDeletesWhenNoHistory(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:delete")
	repo := newFakeComposeFileRepo()
	repo.files["cf-1"] = &domain.ComposeFile{ID: "cf-1", OrganizationID: "org-1"}
	svc := application.NewDeleteComposeFileService(repo, membership, permChecker)

	if err := svc.Execute(context.Background(), "org-1", "user-1", "cf-1"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := repo.files["cf-1"]; ok {
		t.Errorf("expected the compose file row to be gone after a real hard delete")
	}
}

func TestDeleteComposeFileService_RealConflictOnHistoryNoFallback(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:delete")
	repo := newFakeComposeFileRepo()
	repo.files["cf-1"] = &domain.ComposeFile{ID: "cf-1", OrganizationID: "org-1"}
	repo.hasHistory["cf-1"] = true
	svc := application.NewDeleteComposeFileService(repo, membership, permChecker)

	err := svc.Execute(context.Background(), "org-1", "user-1", "cf-1")
	if !errors.Is(err, domain.ErrComposeFileHasHistory) {
		t.Fatalf("expected ErrComposeFileHasHistory to propagate as a real error (no archive concept to fall back to), got: %v", err)
	}
	if _, ok := repo.files["cf-1"]; !ok {
		t.Errorf("expected the compose file row to still exist - a blocked delete must not remove it")
	}
}

func TestDeleteComposeFileService_RequiresComposeFileDelete(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	// Granting only compose_file:manage (not compose_file:delete) must not
	// be enough - the whole point of the new permission is that manage no
	// longer implies delete.
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:manage")
	repo := newFakeComposeFileRepo()
	repo.files["cf-1"] = &domain.ComposeFile{ID: "cf-1", OrganizationID: "org-1"}
	svc := application.NewDeleteComposeFileService(repo, membership, permChecker)

	err := svc.Execute(context.Background(), "org-1", "user-1", "cf-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden with only compose_file:manage (not compose_file:delete), got: %v", err)
	}
}
