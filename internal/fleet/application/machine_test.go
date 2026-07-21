package application_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/envelope"
)

// testMasterKey - a real 32-byte envelope.KeySize key, fixed so tests
// are deterministic (envelope.Seal itself still generates a fresh
// random salt/nonce per call).
var testMasterKey = bytes.Repeat([]byte("k"), 32)

type fakeSecretMountChecker struct {
	mounts map[string]bool // "orgID|mountID"
}

func newFakeSecretMountChecker() *fakeSecretMountChecker {
	return &fakeSecretMountChecker{mounts: map[string]bool{}}
}
func (f *fakeSecretMountChecker) add(orgID, mountID string) {
	f.mounts[orgID+"|"+mountID] = true
}
func (f *fakeSecretMountChecker) SecretMountExists(ctx context.Context, organizationID, mountID string) (bool, error) {
	return f.mounts[organizationID+"|"+mountID], nil
}

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

func TestCreateMachineService_LocalStorage_SealsAndSkipsMountCheck(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	mountCheck := newFakeSecretMountChecker() // deliberately empty - local storage must never call this
	repo := newFakeMachineRepo()
	svc := application.NewCreateMachineService(repo, membership, permChecker, mountCheck, testMasterKey)

	machine, err := svc.Execute(context.Background(), application.CreateMachineInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", Name: "vm1", Host: "10.0.0.5",
		SSHPort: 22, SSHUser: "deploy", CredentialType: "ssh_password", CredentialStorage: "local",
		CredentialSecret: "hunter2", DeployBasePath: "/opt/fleet",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if machine.CredentialStorage != domain.CredentialStorageLocal {
		t.Fatalf("expected CredentialStorageLocal, got %q", machine.CredentialStorage)
	}
	if len(machine.EncryptedCredential) == 0 || len(machine.CredentialNonce) == 0 || len(machine.CredentialSalt) == 0 {
		t.Fatalf("expected sealed credential bytes to be populated")
	}
	if bytes.Contains(machine.EncryptedCredential, []byte("hunter2")) {
		t.Errorf("expected the plaintext secret to never appear verbatim in the stored ciphertext")
	}
	stored := repo.machines[machine.ID]
	if stored == nil || stored.CredentialStorage != domain.CredentialStorageLocal {
		t.Fatalf("expected the machine to be persisted with local storage, got %+v", stored)
	}
}

func TestCreateMachineService_VaultStorage_StillValidatesMount(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	mountCheck := newFakeSecretMountChecker() // deliberately no "mount-1" added
	repo := newFakeMachineRepo()
	svc := application.NewCreateMachineService(repo, membership, permChecker, mountCheck, testMasterKey)

	_, err := svc.Execute(context.Background(), application.CreateMachineInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", Name: "vm1", Host: "10.0.0.5",
		SSHPort: 22, SSHUser: "deploy", CredentialType: "ssh_password", CredentialStorage: "vault",
		CredentialMountID: "mount-1", CredentialPath: "secret/data/x", DeployBasePath: "/opt/fleet",
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a validation error for an unknown mount id, got: %v", err)
	}

	mountCheck.add("org-1", "mount-1")
	machine, err := svc.Execute(context.Background(), application.CreateMachineInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", Name: "vm1", Host: "10.0.0.5",
		SSHPort: 22, SSHUser: "deploy", CredentialType: "ssh_password", CredentialStorage: "vault",
		CredentialMountID: "mount-1", CredentialPath: "secret/data/x", DeployBasePath: "/opt/fleet",
	})
	if err != nil {
		t.Fatalf("expected creation to succeed once the mount is real, got: %v", err)
	}
	if machine.CredentialStorage != domain.CredentialStorageVault {
		t.Errorf("expected CredentialStorageVault, got %q", machine.CredentialStorage)
	}
	if len(machine.EncryptedCredential) != 0 {
		t.Errorf("expected no sealed bytes for a vault-storage machine")
	}
}

// TestCheckMachineConnectionService_ResolvesCredentialByStorage exercises
// resolveMachineCredential (unexported, package application) indirectly
// through CheckMachineConnectionService.Execute - the only way to reach
// it from this external test package - covering both branches: "vault"
// resolves via SecretResolver, "local" opens the sealed bytes and hands
// the SSH probe back the exact original plaintext.
func TestCheckMachineConnectionService_ResolvesCredentialByStorage(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:read")

	t.Run("vault", func(t *testing.T) {
		repo := newFakeMachineRepo()
		repo.machines["m-vault"] = &domain.Machine{
			ID: "m-vault", OrganizationID: "org-1", CredentialStorage: domain.CredentialStorageVault,
			CredentialRef: domain.SecretReference{MountID: "mount-1", Path: "secret/data/x"},
		}
		secretResolver := &fakeDeploySecretResolver{values: map[string]string{"mount-1|secret/data/x": "vault-secret-value"}}
		ssh := &fakeSSHRunner{}
		svc := application.NewCheckMachineConnectionService(repo, membership, permChecker, secretResolver, ssh, testMasterKey)

		if _, err := svc.Execute(context.Background(), "org-1", "user-1", "m-vault"); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ssh.lastTarget.Secret != "vault-secret-value" {
			t.Errorf("expected the vault-resolved secret to reach the SSH probe, got %q", ssh.lastTarget.Secret)
		}
	})

	t.Run("local", func(t *testing.T) {
		sealed, err := envelope.Seal(testMasterKey, []byte("local-secret-value"))
		if err != nil {
			t.Fatalf("envelope.Seal: %v", err)
		}
		repo := newFakeMachineRepo()
		repo.machines["m-local"] = &domain.Machine{
			ID: "m-local", OrganizationID: "org-1", CredentialStorage: domain.CredentialStorageLocal,
			EncryptedCredential: sealed.Ciphertext, CredentialNonce: sealed.Nonce, CredentialSalt: sealed.Salt,
		}
		secretResolver := &fakeDeploySecretResolver{values: map[string]string{}} // deliberately empty - local must never call this
		ssh := &fakeSSHRunner{}
		svc := application.NewCheckMachineConnectionService(repo, membership, permChecker, secretResolver, ssh, testMasterKey)

		if _, err := svc.Execute(context.Background(), "org-1", "user-1", "m-local"); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if ssh.lastTarget.Secret != "local-secret-value" {
			t.Errorf("expected the unsealed local secret to reach the SSH probe, got %q", ssh.lastTarget.Secret)
		}
	})
}

func TestDuplicateMachineService_CreatesCloneWithGivenName(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{
		ID: "m-1", OrganizationID: "org-1", Name: "develop", Host: "10.0.0.5",
		CredentialStorage: domain.CredentialStorageVault,
		CredentialRef:     domain.SecretReference{MountID: "mount-1", Path: "secret/data/x"},
		Archived:          false,
	}
	svc := application.NewDuplicateMachineService(repo, membership, permChecker)

	clone, err := svc.Execute(context.Background(), application.DuplicateMachineInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", MachineID: "m-1", Name: "develop (copy)",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if clone.Name != "develop (copy)" {
		t.Errorf("expected the exact operator-supplied name, got %q", clone.Name)
	}
	if clone.ID == "m-1" {
		t.Errorf("expected a new ID, got the source's own ID")
	}
	if clone.CredentialRef.MountID != "mount-1" || clone.CredentialRef.Path != "secret/data/x" {
		t.Errorf("expected the vault credential reference to be copied verbatim, got %+v", clone.CredentialRef)
	}
	if _, ok := repo.machines["m-1"]; !ok {
		t.Errorf("expected the source machine to still exist untouched")
	}
}

func TestDuplicateMachineService_ConflictOnExplicitNameIsARealError(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1", Name: "develop"}
	repo.machines["m-2"] = &domain.Machine{ID: "m-2", OrganizationID: "org-1", Name: "develop (copy)"}
	svc := application.NewDuplicateMachineService(repo, membership, permChecker)

	_, err := svc.Execute(context.Background(), application.DuplicateMachineInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", MachineID: "m-1", Name: "develop (copy)",
	})
	if !errors.Is(err, domain.ErrMachineNameTaken) {
		t.Fatalf("expected a real ErrMachineNameTaken conflict (no silent rename), got: %v", err)
	}
}

func TestDuplicateMachineService_AutoGeneratesNameWhenOmitted(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "machine:manage")
	repo := newFakeMachineRepo()
	repo.machines["m-1"] = &domain.Machine{ID: "m-1", OrganizationID: "org-1", Name: "develop"}
	svc := application.NewDuplicateMachineService(repo, membership, permChecker)

	clone, err := svc.Execute(context.Background(), application.DuplicateMachineInput{
		OrganizationID: "org-1", RequestingUserID: "user-1", MachineID: "m-1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if clone.Name != "develop (copy)" {
		t.Errorf("expected auto-generated name %q, got %q", "develop (copy)", clone.Name)
	}
}
