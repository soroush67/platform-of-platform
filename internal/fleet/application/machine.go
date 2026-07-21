package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/envelope"
)

type CreateMachineInput struct {
	OrganizationID    string
	RequestingUserID  string
	Name              string
	Host              string
	SSHPort           int
	SSHUser           string
	CredentialType    string
	CredentialStorage string
	// CredentialMountID/CredentialPath are used when CredentialStorage
	// is "vault" (the default, matching every Machine before this field
	// existed).
	CredentialMountID string
	CredentialPath    string
	// CredentialSecret is the plaintext SSH secret, used when
	// CredentialStorage is "local" - exists as a Go value only for the
	// duration of this call (sealed via envelope.Seal below, never
	// logged, never returned), same posture CreateSecretMountService's
	// own SecretID field already documents.
	CredentialSecret string
	DeployBasePath   string
}

type CreateMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	mountCheck  SecretMountChecker
	masterKey   []byte
}

func NewCreateMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker, masterKey []byte) *CreateMachineService {
	return &CreateMachineService{repo: repo, membership: membership, permChecker: permChecker, mountCheck: mountCheck, masterKey: masterKey}
}

func (s *CreateMachineService) Execute(ctx context.Context, in CreateMachineInput) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionMachineManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	storage := domain.CredentialStorage(in.CredentialStorage)
	if !storage.Valid() {
		return nil, &domain.ValidationError{Message: "credential_storage must be one of vault, local"}
	}

	var machine *domain.Machine
	if storage == domain.CredentialStorageVault {
		exists, err := s.mountCheck.SecretMountExists(ctx, in.OrganizationID, in.CredentialMountID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "credential_mount_id does not reference a real secret mount in this organization"}
		}

		machine, err = domain.NewMachine(in.OrganizationID, in.Name, in.Host, in.SSHPort, in.SSHUser,
			domain.CredentialType(in.CredentialType),
			domain.SecretReference{MountID: in.CredentialMountID, Path: in.CredentialPath},
			in.DeployBasePath,
		)
		if err != nil {
			return nil, err
		}
	} else {
		if in.CredentialSecret == "" {
			return nil, &domain.ValidationError{Message: "credential_secret is required for local credential storage"}
		}
		sealed, err := envelope.Seal(s.masterKey, []byte(in.CredentialSecret))
		if err != nil {
			return nil, err
		}
		machine, err = domain.NewMachineWithLocalCredential(in.OrganizationID, in.Name, in.Host, in.SSHPort, in.SSHUser,
			domain.CredentialType(in.CredentialType),
			sealed.Ciphertext, sealed.Nonce, sealed.Salt,
			in.DeployBasePath,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := s.repo.Create(ctx, in.RequestingUserID, machine); err != nil {
		return nil, err
	}
	return machine, nil
}

type ListMachinesService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewListMachinesService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *ListMachinesService {
	return &ListMachinesService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *ListMachinesService) Execute(ctx context.Context, organizationID, requestingUserID string, includeArchived bool) ([]*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionMachineRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListByOrganization(ctx, organizationID, includeArchived)
}

type GetMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewGetMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *GetMachineService {
	return &GetMachineService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *GetMachineService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionMachineRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.GetByID(ctx, organizationID, machineID)
}

type UpdateMachineInput struct {
	OrganizationID    string
	RequestingUserID  string
	MachineID         string
	SSHUser           *string
	CredentialType    *string
	CredentialMountID *string
	CredentialPath    *string
	// CredentialSecret re-seals and overwrites a CredentialStorageLocal
	// Machine's own stored secret - only meaningful for a Machine
	// already using local storage; switching storage type on an
	// existing Machine is out of scope (not asked for).
	CredentialSecret *string
	DeployBasePath   *string
}

type UpdateMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	mountCheck  SecretMountChecker
	masterKey   []byte
}

func NewUpdateMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker, masterKey []byte) *UpdateMachineService {
	return &UpdateMachineService{repo: repo, membership: membership, permChecker: permChecker, mountCheck: mountCheck, masterKey: masterKey}
}

func (s *UpdateMachineService) Execute(ctx context.Context, in UpdateMachineInput) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionMachineManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	machine, err := s.repo.GetByID(ctx, in.OrganizationID, in.MachineID)
	if err != nil {
		return nil, err
	}

	if in.SSHUser != nil {
		machine.SSHUser = *in.SSHUser
	}
	if in.DeployBasePath != nil {
		machine.DeployBasePath = *in.DeployBasePath
	}
	if in.CredentialType != nil {
		ct := domain.CredentialType(*in.CredentialType)
		if !ct.Valid() {
			return nil, &domain.ValidationError{Message: "invalid credential_type"}
		}
		machine.CredentialType = ct
	}
	if in.CredentialMountID != nil || in.CredentialPath != nil {
		mountID := machine.CredentialRef.MountID
		path := machine.CredentialRef.Path
		if in.CredentialMountID != nil {
			mountID = *in.CredentialMountID
		}
		if in.CredentialPath != nil {
			path = *in.CredentialPath
		}
		exists, err := s.mountCheck.SecretMountExists(ctx, in.OrganizationID, mountID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "credential_mount_id does not reference a real secret mount in this organization"}
		}
		machine.CredentialRef = domain.SecretReference{MountID: mountID, Path: path}
	}
	if in.CredentialSecret != nil {
		if machine.CredentialStorage != domain.CredentialStorageLocal {
			return nil, &domain.ValidationError{Message: "credential_secret can only be updated for a Machine using local credential storage"}
		}
		sealed, err := envelope.Seal(s.masterKey, []byte(*in.CredentialSecret))
		if err != nil {
			return nil, err
		}
		machine.EncryptedCredential = sealed.Ciphertext
		machine.CredentialNonce = sealed.Nonce
		machine.CredentialSalt = sealed.Salt
	}

	if err := s.repo.Update(ctx, in.RequestingUserID, machine); err != nil {
		return nil, err
	}
	return machine, nil
}

// ArchiveMachineService is a pure, reversible soft-archive - never
// attempts a hard delete. Gated by machine:manage (the same tier every
// other reversible Machine action uses) - DeleteMachineService below is
// the genuinely destructive, Owner-only counterpart, a separate action
// entirely (operator's own explicit choice: two distinct buttons, not
// one action that silently decides between them).
type ArchiveMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewArchiveMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *ArchiveMachineService {
	return &ArchiveMachineService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *ArchiveMachineService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionMachineManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	return s.repo.Archive(ctx, requestingUserID, organizationID, machineID)
}

// UnarchiveMachineService reverses ArchiveMachineService - same
// machine:manage tier, same pure/reversible posture, real gap the
// operator reported (archiving a Machine was previously a one-way door
// in this UI, even though the repo/domain state was always reversible).
type UnarchiveMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewUnarchiveMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *UnarchiveMachineService {
	return &UnarchiveMachineService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *UnarchiveMachineService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionMachineManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	return s.repo.Unarchive(ctx, requestingUserID, organizationID, machineID)
}

// DeleteMachineService is the genuinely destructive, Owner-only
// counterpart to ArchiveMachineService above - a real hard delete only,
// no silent archive fallback. repo.Delete already maps a real FK
// violation (the Machine has real Operation history) to
// domain.ErrMachineHasHistory - that propagates straight to the caller
// as a real 409 here, letting the operator choose Archive instead
// themselves rather than the service silently deciding for them.
type DeleteMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDeleteMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *DeleteMachineService {
	return &DeleteMachineService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *DeleteMachineService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionMachineDelete)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	return s.repo.Delete(ctx, requestingUserID, organizationID, machineID)
}

// DuplicateMachineInput - Name is optional: the UI always sends the
// proposed "{name} (copy)" it shows the operator for confirmation/edit
// before creating (operator's own explicit ask - see a real name
// up front, not a silent auto-generated one), but a caller that omits
// it (e.g. a future API consumer) still gets a real, auto-generated
// unique name rather than a required-field error.
type DuplicateMachineInput struct {
	OrganizationID   string
	RequestingUserID string
	MachineID        string
	Name             string
}

// DuplicateMachineService clones an existing Machine into a brand new
// one - host/port/user/credential_type/deploy_base_path copied
// verbatim, and the credential itself copied WITHOUT ever touching
// plaintext: a vault-storage Machine's mount_id+path are just a
// reference (never sensitive on their own), and a local-storage
// Machine's already-sealed EncryptedCredential/CredentialNonce/
// CredentialSalt can be reused as-is - envelope.Open/Seal is never
// called here.
type DuplicateMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDuplicateMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *DuplicateMachineService {
	return &DuplicateMachineService{repo: repo, membership: membership, permChecker: permChecker}
}

const maxDuplicateNameAttempts = 50

func (s *DuplicateMachineService) Execute(ctx context.Context, in DuplicateMachineInput) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionMachineManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	source, err := s.repo.GetByID(ctx, in.OrganizationID, in.MachineID)
	if err != nil {
		return nil, err
	}

	newMachine := func(name string) *domain.Machine {
		clone := *source
		clone.ID = uuid.NewString()
		clone.Name = name
		clone.ConnectionStatus = domain.ConnectionStatusUnknown
		clone.DockerStatus = domain.DockerStatusUnknown
		clone.LastCheckedAt = nil
		clone.Archived = false
		clone.CreatedAt = time.Now().UTC()
		return &clone
	}

	// An explicit name (the normal path - the UI always sends one) is
	// tried exactly once - a conflict is the operator's own choice to
	// resolve (real 409), not something this service silently works
	// around behind their back.
	if in.Name != "" {
		clone := newMachine(in.Name)
		if err := s.repo.Create(ctx, in.RequestingUserID, clone); err != nil {
			return nil, err
		}
		return clone, nil
	}

	for attempt := 1; attempt <= maxDuplicateNameAttempts; attempt++ {
		clone := newMachine(duplicateName(source.Name, attempt))
		err := s.repo.Create(ctx, in.RequestingUserID, clone)
		if err == nil {
			return clone, nil
		}
		if !errors.Is(err, domain.ErrMachineNameTaken) {
			return nil, err
		}
	}
	return nil, &domain.ValidationError{Message: "could not find an available name to duplicate this machine under"}
}

func duplicateName(name string, attempt int) string {
	if attempt == 1 {
		return name + " (copy)"
	}
	return fmt.Sprintf("%s (copy %d)", name, attempt)
}

// TestMachineConnectionInput/TestMachineConnectionService probe
// connectivity+docker over SSH WITHOUT a saved Machine row - lets an
// admin verify a host/credential pair before committing to
// CreateMachine, same real precedent the ported Python product's own
// POST /machines/test-connection set.
type TestMachineConnectionInput struct {
	OrganizationID    string
	RequestingUserID  string
	Host              string
	SSHPort           int
	SSHUser           string
	CredentialType    string
	CredentialStorage string
	CredentialMountID string
	CredentialPath    string
	// CredentialSecret is the plaintext secret to probe with directly
	// when CredentialStorage is "local" - there's no saved Machine row
	// yet at this point, so there's nothing to seal/unseal, unlike
	// resolveMachineCredential below.
	CredentialSecret string
}

type TestMachineConnectionService struct {
	membership     MembershipChecker
	permChecker    PermissionChecker
	secretResolver SecretResolver
	ssh            SSHRunner
}

func NewTestMachineConnectionService(membership MembershipChecker, permChecker PermissionChecker, secretResolver SecretResolver, ssh SSHRunner) *TestMachineConnectionService {
	return &TestMachineConnectionService{membership: membership, permChecker: permChecker, secretResolver: secretResolver, ssh: ssh}
}

func (s *TestMachineConnectionService) Execute(ctx context.Context, in TestMachineConnectionInput) (domain.ConnectionStatus, domain.DockerStatus, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return "", "", err
	}
	if !isMember {
		return "", "", domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionMachineManage)
	if err != nil {
		return "", "", err
	}
	if !allowed {
		return "", "", domain.ErrForbidden
	}

	var secret string
	if domain.CredentialStorage(in.CredentialStorage) == domain.CredentialStorageLocal {
		secret = in.CredentialSecret
	} else {
		secret, err = s.secretResolver.ResolveValue(ctx, in.OrganizationID, in.CredentialMountID, in.CredentialPath)
		if err != nil {
			return "", "", err
		}
	}

	return s.ssh.Probe(ctx, ConnectionTarget{
		Host: in.Host, Port: in.SSHPort, User: in.SSHUser,
		CredentialType: domain.CredentialType(in.CredentialType), Secret: secret,
	})
}

// CheckMachineConnectionService is the saved-Machine counterpart -
// probes and PERSISTS the result (RecordConnectionCheck), unlike
// TestMachineConnectionService above which only ever reports back,
// never writes anything.
type CheckMachineConnectionService struct {
	repo           MachineRepository
	membership     MembershipChecker
	permChecker    PermissionChecker
	secretResolver SecretResolver
	ssh            SSHRunner
	masterKey      []byte
	now            func() time.Time
}

func NewCheckMachineConnectionService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker, secretResolver SecretResolver, ssh SSHRunner, masterKey []byte) *CheckMachineConnectionService {
	return &CheckMachineConnectionService{repo: repo, membership: membership, permChecker: permChecker, secretResolver: secretResolver, ssh: ssh, masterKey: masterKey, now: time.Now}
}

func (s *CheckMachineConnectionService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	// fleet:read, matching the Python original's own require_machine_access
	// (any grantee, not admin/manage-only) for this specific action.
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionMachineRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	machine, err := s.repo.GetByID(ctx, organizationID, machineID)
	if err != nil {
		return nil, err
	}

	secret, err := resolveMachineCredential(ctx, s.secretResolver, s.masterKey, machine)
	if err != nil {
		return nil, err
	}

	connStatus, dockerStatus, probeErr := s.ssh.Probe(ctx, ConnectionTarget{
		Host: machine.Host, Port: machine.SSHPort, User: machine.SSHUser,
		CredentialType: machine.CredentialType, Secret: secret,
	})
	if probeErr != nil {
		// A probe failure is still a real, reportable result
		// (unreachable/unknown), not a request failure - matches the
		// Python original's own check_connection, which never lets an
		// SSH-layer error escape as a 5xx.
		connStatus, dockerStatus = domain.ConnectionStatusUnreachable, domain.DockerStatusUnknown
	}
	machine.RecordConnectionCheck(connStatus, dockerStatus, s.now().UTC())

	if err := s.repo.Update(ctx, requestingUserID, machine); err != nil {
		return nil, err
	}
	return machine, nil
}

// resolveMachineCredential branches on a Machine's own CredentialStorage -
// "vault" resolves live via SecretResolver (unchanged, original path);
// "local" opens the sealed bytes already stored on the Machine itself via
// envelope.Open, no live Vault call at all. Shared by
// CheckMachineConnectionService and DeployExecutor, both of which
// already hold a real *domain.Machine (unlike TestMachineConnectionService,
// which runs before any Machine row exists and branches inline instead).
func resolveMachineCredential(ctx context.Context, secretResolver SecretResolver, masterKey []byte, m *domain.Machine) (string, error) {
	if m.CredentialStorage == domain.CredentialStorageLocal {
		plaintext, err := envelope.Open(masterKey, &envelope.Sealed{
			Ciphertext: m.EncryptedCredential,
			Nonce:      m.CredentialNonce,
			Salt:       m.CredentialSalt,
		})
		if err != nil {
			return "", err
		}
		return string(plaintext), nil
	}
	return secretResolver.ResolveValue(ctx, m.OrganizationID, m.CredentialRef.MountID, m.CredentialRef.Path)
}
