package application

import (
	"context"
	"errors"
	"time"

	"platform-of-platform/internal/fleet/domain"
)

type CreateMachineInput struct {
	OrganizationID    string
	RequestingUserID  string
	Name              string
	Host              string
	SSHPort           int
	SSHUser           string
	CredentialType    string
	CredentialMountID string
	CredentialPath    string
	DeployBasePath    string
}

type CreateMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	mountCheck  SecretMountChecker
}

func NewCreateMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker) *CreateMachineService {
	return &CreateMachineService{repo: repo, membership: membership, permChecker: permChecker, mountCheck: mountCheck}
}

func (s *CreateMachineService) Execute(ctx context.Context, in CreateMachineInput) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionFleetManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	exists, err := s.mountCheck.SecretMountExists(ctx, in.OrganizationID, in.CredentialMountID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &domain.ValidationError{Message: "credential_mount_id does not reference a real secret mount in this organization"}
	}

	machine, err := domain.NewMachine(in.OrganizationID, in.Name, in.Host, in.SSHPort, in.SSHUser,
		domain.CredentialType(in.CredentialType),
		domain.SecretReference{MountID: in.CredentialMountID, Path: in.CredentialPath},
		in.DeployBasePath,
	)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, in.RequestingUserID, machine); err != nil {
		return nil, err
	}
	return machine, nil
}

type ListMachinesService struct {
	repo       MachineRepository
	membership MembershipChecker
}

func NewListMachinesService(repo MachineRepository, membership MembershipChecker) *ListMachinesService {
	return &ListMachinesService{repo: repo, membership: membership}
}

func (s *ListMachinesService) Execute(ctx context.Context, organizationID, requestingUserID string, includeArchived bool) ([]*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListByOrganization(ctx, organizationID, includeArchived)
}

type GetMachineService struct {
	repo       MachineRepository
	membership MembershipChecker
}

func NewGetMachineService(repo MachineRepository, membership MembershipChecker) *GetMachineService {
	return &GetMachineService{repo: repo, membership: membership}
}

func (s *GetMachineService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
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
	DeployBasePath    *string
}

type UpdateMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	mountCheck  SecretMountChecker
}

func NewUpdateMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker) *UpdateMachineService {
	return &UpdateMachineService{repo: repo, membership: membership, permChecker: permChecker, mountCheck: mountCheck}
}

func (s *UpdateMachineService) Execute(ctx context.Context, in UpdateMachineInput) (*domain.Machine, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionFleetManage)
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

	if err := s.repo.Update(ctx, in.RequestingUserID, machine); err != nil {
		return nil, err
	}
	return machine, nil
}

// ArchiveMachineService performs a real hard delete, falling back to a
// soft archive on domain.ErrMachineHasHistory (a real FK violation
// against real Operation rows) - exact same delete-or-archive-fallback
// behavior as the ported Python product's own DELETE /machines/{id}.
type ArchiveMachineService struct {
	repo        MachineRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewArchiveMachineService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *ArchiveMachineService {
	return &ArchiveMachineService{repo: repo, membership: membership, permChecker: permChecker}
}

// Execute returns (archived bool, err) - archived=true tells the HTTP
// handler to report "archived, not deleted" distinctly from a clean
// hard delete, matching the Python original's own two-outcome response.
func (s *ArchiveMachineService) Execute(ctx context.Context, organizationID, requestingUserID, machineID string) (archived bool, err error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return false, err
	}
	if !isMember {
		return false, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionFleetManage)
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, domain.ErrForbidden
	}

	err = s.repo.Delete(ctx, requestingUserID, organizationID, machineID)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, domain.ErrMachineHasHistory) {
		if archiveErr := s.repo.Archive(ctx, requestingUserID, organizationID, machineID); archiveErr != nil {
			return false, archiveErr
		}
		return true, nil
	}
	return false, err
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
	CredentialMountID string
	CredentialPath    string
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
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionFleetManage)
	if err != nil {
		return "", "", err
	}
	if !allowed {
		return "", "", domain.ErrForbidden
	}

	secret, err := s.secretResolver.ResolveValue(ctx, in.OrganizationID, in.CredentialMountID, in.CredentialPath)
	if err != nil {
		return "", "", err
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
	now            func() time.Time
}

func NewCheckMachineConnectionService(repo MachineRepository, membership MembershipChecker, permChecker PermissionChecker, secretResolver SecretResolver, ssh SSHRunner) *CheckMachineConnectionService {
	return &CheckMachineConnectionService{repo: repo, membership: membership, permChecker: permChecker, secretResolver: secretResolver, ssh: ssh, now: time.Now}
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
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionFleetRead)
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

	secret, err := s.secretResolver.ResolveValue(ctx, organizationID, machine.CredentialRef.MountID, machine.CredentialRef.Path)
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
