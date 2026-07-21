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

	return s.WriteValue(ctx, in.OrganizationID, in.MountID, in.Path, in.Value)
}

// WriteValue - no RBAC of its own, same trusted-cross-context-port
// pattern as ResolveSecretService.ResolveValue (resolve_secret.go): the
// caller has already gated on its own scope's permission before ever
// reaching this, so a redundant organization:manage check here
// wouldn't fit a narrower caller (e.g. Fleet's own compose_file:manage-
// gated vault-backed Variable creation). Execute (this service's own
// HTTP-facing entrypoint, used directly by POST .../secret-mounts/{id}/
// secrets) still does the full organization:manage check above before
// ever calling this.
func (s *WriteSecretService) WriteValue(ctx context.Context, organizationID, mountID, path, value string) error {
	mount, err := s.repo.GetByID(ctx, organizationID, mountID)
	if err != nil {
		return err
	}

	secretID, err := decryptSecretID(s.masterKey, mount)
	if err != nil {
		return err
	}

	switch mount.BackendType {
	case domain.BackendTypeVault:
		return s.vault.WriteSecret(ctx, mount.Address, mount.RoleID, secretID, path, value)
	default:
		return &domain.ValidationError{Message: "backend_type " + string(mount.BackendType) + " is not yet implemented"}
	}
}
