package application_test

import (
	"context"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

type fakeSecretWriter struct {
	writes []fakeSecretWrite
	fail   bool
}

type fakeSecretWrite struct {
	organizationID, mountID, path, value string
}

func (f *fakeSecretWriter) WriteValue(ctx context.Context, organizationID, mountID, path, value string) error {
	if f.fail {
		return errFakeSecretWriter
	}
	f.writes = append(f.writes, fakeSecretWrite{organizationID, mountID, path, value})
	return nil
}

var errFakeSecretWriter = &domain.ValidationError{Message: "fake secret writer failure"}

func TestCreateSecretVariableService_HappyPath_WritesOnceProjectLinkAndCreatesVariable(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:manage")
	mountCheck := newFakeSecretMountChecker()
	mountCheck.add("org-1", "mount-1")
	attachments := newFakeProjectLinkRepo()
	attachments.AttachProject(context.Background(), "user-1", "org-1", "cf-1", "project-b")
	attachments.AttachProject(context.Background(), "user-1", "org-1", "cf-1", "project-a")
	writer := &fakeSecretWriter{}
	repo := newFakeVariableRepo()

	svc := application.NewCreateSecretVariableService(repo, attachments, membership, permChecker, mountCheck, writer)
	variable, err := svc.Execute(context.Background(), application.CreateSecretVariableInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", ComposeFileID: "cf-1",
		Key: "DB_PASSWORD", MountID: "mount-1", Value: "super-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if variable.VarType != domain.VarTypeSecret {
		t.Fatalf("expected var_type secret, got %s", variable.VarType)
	}
	if len(writer.writes) != 2 {
		t.Fatalf("expected 2 vault writes (one per linked project), got %d", len(writer.writes))
	}
	for _, w := range writer.writes {
		if w.value != "super-secret" || w.mountID != "mount-1" || w.organizationID != "org-1" {
			t.Fatalf("unexpected write: %+v", w)
		}
	}
	// project-a sorts before project-b - the canonical stored path must
	// deterministically pick the lexicographically-first project ID.
	wantPath := "secret/data/fleet/compose-files/project-a/cf-1/DB_PASSWORD"
	if variable.SecretRef == nil || variable.SecretRef.Path != wantPath {
		t.Fatalf("expected canonical path %q, got %+v", wantPath, variable.SecretRef)
	}
	if writer.writes[0].path != wantPath {
		t.Fatalf("expected first write to use the canonical path %q, got %q", wantPath, writer.writes[0].path)
	}
}

func TestCreateSecretVariableService_NoLinkedProjects_ReturnsValidationError(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:manage")
	mountCheck := newFakeSecretMountChecker()
	mountCheck.add("org-1", "mount-1")
	attachments := newFakeProjectLinkRepo()
	writer := &fakeSecretWriter{}
	repo := newFakeVariableRepo()

	svc := application.NewCreateSecretVariableService(repo, attachments, membership, permChecker, mountCheck, writer)
	_, err := svc.Execute(context.Background(), application.CreateSecretVariableInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", ComposeFileID: "cf-1",
		Key: "DB_PASSWORD", MountID: "mount-1", Value: "super-secret",
	})
	var validationErr *domain.ValidationError
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !isValidationError(err, &validationErr) {
		t.Fatalf("expected a *domain.ValidationError, got %T: %v", err, err)
	}
	if len(writer.writes) != 0 {
		t.Fatalf("expected zero vault writes, got %d", len(writer.writes))
	}
}

func TestCreateSecretVariableService_MountDoesNotExist_ReturnsValidationError(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:manage")
	mountCheck := newFakeSecretMountChecker() // no mounts registered
	attachments := newFakeProjectLinkRepo()
	attachments.AttachProject(context.Background(), "user-1", "org-1", "cf-1", "project-a")
	writer := &fakeSecretWriter{}
	repo := newFakeVariableRepo()

	svc := application.NewCreateSecretVariableService(repo, attachments, membership, permChecker, mountCheck, writer)
	_, err := svc.Execute(context.Background(), application.CreateSecretVariableInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", ComposeFileID: "cf-1",
		Key: "DB_PASSWORD", MountID: "does-not-exist", Value: "super-secret",
	})
	var validationErr *domain.ValidationError
	if !isValidationError(err, &validationErr) {
		t.Fatalf("expected a *domain.ValidationError, got %T: %v", err, err)
	}
}

func TestCreateSecretVariableService_NotMember_ReturnsForbidden(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker() // no one added
	permChecker := newFakeAttachmentPermChecker()
	mountCheck := newFakeSecretMountChecker()
	attachments := newFakeProjectLinkRepo()
	writer := &fakeSecretWriter{}
	repo := newFakeVariableRepo()

	svc := application.NewCreateSecretVariableService(repo, attachments, membership, permChecker, mountCheck, writer)
	_, err := svc.Execute(context.Background(), application.CreateSecretVariableInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", ComposeFileID: "cf-1",
		Key: "DB_PASSWORD", MountID: "mount-1", Value: "super-secret",
	})
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}
