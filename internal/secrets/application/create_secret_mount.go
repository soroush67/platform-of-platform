package application

import (
	"context"

	"platform-of-platform/internal/platform/envelope"
	"platform-of-platform/internal/secrets/domain"
)

const permissionOrganizationManage = "organization:manage"

// CreateSecretMountInput implements
// `POST /orgs/{org}/secret-mounts` (docs/architecture/11-module-
// secrets-state.md §1). SecretID is the plaintext AppRole secret_id -
// it exists as a Go value only for the duration of this call (read from
// the request body, handed to envelope.Seal, then discarded); it's
// never logged, never returned, never held anywhere past Execute
// returning.
type CreateSecretMountInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	BackendType      string
	Address          string
	RoleID           string
	SecretID         string
}

type CreateSecretMountService struct {
	repo        SecretMountRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	masterKey   []byte
}

func NewCreateSecretMountService(repo SecretMountRepository, membership MembershipChecker, permChecker PermissionChecker, masterKey []byte) *CreateSecretMountService {
	return &CreateSecretMountService{repo: repo, membership: membership, permChecker: permChecker, masterKey: masterKey}
}

func (s *CreateSecretMountService) Execute(ctx context.Context, in CreateSecretMountInput) (*domain.SecretMount, error) {
	// Membership then permission, same ordering (and same ErrForbidden-
	// for-a-non-member choice, since this context has no "organization
	// not found" sentinel of its own) as rbac.CreateRoleService.
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	backendType := domain.BackendType(in.BackendType)
	if backendType != domain.BackendTypeVault {
		// See BackendType's own doc comment - modeled fully, only Vault
		// is actually wired to a real adapter.
		return nil, &domain.ValidationError{Message: "backend_type " + in.BackendType + " is not yet implemented - only \"vault\" is supported"}
	}
	if in.SecretID == "" {
		return nil, &domain.ValidationError{Message: "secret_id is required"}
	}

	sealed, err := envelope.Seal(s.masterKey, []byte(in.SecretID))
	if err != nil {
		return nil, err
	}

	mount, err := domain.NewSecretMount(in.OrganizationID, in.Name, backendType, in.Address, in.RoleID, sealed.Ciphertext, sealed.Nonce, sealed.Salt)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, mount); err != nil {
		return nil, err
	}

	return mount, nil
}
