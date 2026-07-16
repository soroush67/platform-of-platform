package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

type TriggerOperationInput struct {
	OrganizationID   string
	RequestingUserID string
	ComposeFileID    string
	MachineID        string
	OperationType    string
}

// TriggerOperationService gates on fleet:deploy specifically - the
// higher-consequence permission (mirrors workspace:apply vs
// workspace:manage) since this is the one action that actually reaches
// out over real SSH to a real remote machine; every other mutating
// Fleet service gates on the lower-consequence fleet:manage.
type TriggerOperationService struct {
	operations   OperationRepository
	composeFiles ComposeFileRepository
	machines     MachineRepository
	membership   MembershipChecker
	permChecker  PermissionChecker
}

func NewTriggerOperationService(operations OperationRepository, composeFiles ComposeFileRepository, machines MachineRepository, membership MembershipChecker, permChecker PermissionChecker) *TriggerOperationService {
	return &TriggerOperationService{operations: operations, composeFiles: composeFiles, machines: machines, membership: membership, permChecker: permChecker}
}

func (s *TriggerOperationService) Execute(ctx context.Context, in TriggerOperationInput) (*domain.Operation, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOperationDeploy)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	// Real existence checks before ever queuing - a queued Operation
	// DeployExecutor can't resolve later isn't a graceful "not found"
	// anymore, it's an internal error mid-flight. Fail fast here
	// instead, same posture as TriggerRunService's own workspace-exists
	// check.
	if _, err := s.composeFiles.GetByID(ctx, in.OrganizationID, in.ComposeFileID); err != nil {
		return nil, err
	}
	machine, err := s.machines.GetByID(ctx, in.OrganizationID, in.MachineID)
	if err != nil {
		return nil, err
	}
	if machine.Archived {
		return nil, &domain.ValidationError{Message: "machine is archived and cannot receive new operations"}
	}

	operation, err := domain.NewOperation(in.OrganizationID, in.ComposeFileID, in.MachineID, domain.OperationType(in.OperationType), in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if err := s.operations.Create(ctx, operation); err != nil {
		return nil, err
	}
	return operation, nil
}

type ListOperationsService struct {
	repo        OperationRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewListOperationsService(repo OperationRepository, membership MembershipChecker, permChecker PermissionChecker) *ListOperationsService {
	return &ListOperationsService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *ListOperationsService) Execute(ctx context.Context, organizationID, requestingUserID, composeFileID, machineID string) ([]*domain.Operation, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionOperationRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListByOrganization(ctx, organizationID, composeFileID, machineID)
}

type GetOperationService struct {
	repo        OperationRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewGetOperationService(repo OperationRepository, membership MembershipChecker, permChecker PermissionChecker) *GetOperationService {
	return &GetOperationService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *GetOperationService) Execute(ctx context.Context, organizationID, requestingUserID, operationID string) (*domain.Operation, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionOperationRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	return s.repo.GetByID(ctx, organizationID, operationID)
}
