package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

// RevokeAPIKeyInput implements
// `DELETE /orgs/{org}/service-accounts/{sa}/api-keys/{key}`.
type RevokeAPIKeyInput struct {
	OrganizationID   string
	RequestingUserID string
	APIKeyID         string
}

type RevokeAPIKeyService struct {
	repo        APIKeyRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewRevokeAPIKeyService(repo APIKeyRepository, membership MembershipChecker, permChecker PermissionChecker) *RevokeAPIKeyService {
	return &RevokeAPIKeyService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *RevokeAPIKeyService) Execute(ctx context.Context, in RevokeAPIKeyInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrAPIKeyInvalid
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	return s.repo.Revoke(ctx, in.OrganizationID, in.APIKeyID)
}
