package application

import (
	"context"

	"platform-of-platform/internal/secrets/domain"
)

// ListSecretMountsService - membership-gated only, any role, same
// posture as every other read in this codebase. Returns the real
// domain.SecretMount (including the sealed credential bytes) the same
// way every other List*Service in this codebase returns its full
// domain type - it's the HTTP adapter's own response DTO, not this
// service, that decides EncryptedSecretID/Nonce/Salt never actually
// serialize out (same "domain type in, DTO shapes the wire response"
// split as everywhere else).
type ListSecretMountsService struct {
	repo       SecretMountRepository
	membership MembershipChecker
}

func NewListSecretMountsService(repo SecretMountRepository, membership MembershipChecker) *ListSecretMountsService {
	return &ListSecretMountsService{repo: repo, membership: membership}
}

func (s *ListSecretMountsService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.SecretMount, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListForOrganization(ctx, organizationID)
}
