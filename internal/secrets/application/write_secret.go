package application

import (
	"context"

	"platform-of-platform/internal/secrets/domain"
)

// WriteSecretInput implements `POST /orgs/{org}/secret-mounts/{mount}/
// secrets` - lets an operator store a credential (e.g. a Fleet Machine's
// SSH password/key) directly through this platform's own API rather
// than requiring an out-of-band `vault kv put` first. Same
// organization:manage gate as TestConnectionService/
// CreateSecretMountService - writing a real secret into the backing
// store is exactly as consequential as creating the mount itself.
type WriteSecretInput struct {
	OrganizationID   string
	MountID          string
	RequestingUserID string
	Path             string
	Value            string
}

type WriteSecretService struct {
	repo        SecretMountRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	vault       VaultClient
	masterKey   []byte
}

func NewWriteSecretService(repo SecretMountRepository, membership MembershipChecker, permChecker PermissionChecker, vault VaultClient, masterKey []byte) *WriteSecretService {
	return &WriteSecretService{repo: repo, membership: membership, permChecker: permChecker, vault: vault, masterKey: masterKey}
}

func (s *WriteSecretService) Execute(ctx context.Context, in WriteSecretInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	if in.Path == "" || in.Value == "" {
		return &domain.ValidationError{Message: "path and value are both required"}
	}

	mount, err := s.repo.GetByID(ctx, in.OrganizationID, in.MountID)
	if err != nil {
		return err
	}

	secretID, err := decryptSecretID(s.masterKey, mount)
	if err != nil {
		return err
	}

	switch mount.BackendType {
	case domain.BackendTypeVault:
		return s.vault.WriteSecret(ctx, mount.Address, mount.RoleID, secretID, in.Path, in.Value)
	default:
		return &domain.ValidationError{Message: "backend_type " + string(mount.BackendType) + " is not yet implemented"}
	}
}
