package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

// ListRoleBindingsService implements
// `GET /orgs/{org}/role-bindings?subject_id=...` - "what can this
// subject do, and where" (docs/architecture/13-module-identity-rbac-
// tenancy.md §3). Empty subjectID lists every binding in the org.
type ListRoleBindingsService struct {
	repo       RoleBindingRepository
	membership MembershipChecker
}

func NewListRoleBindingsService(repo RoleBindingRepository, membership MembershipChecker) *ListRoleBindingsService {
	return &ListRoleBindingsService{repo: repo, membership: membership}
}

func (s *ListRoleBindingsService) Execute(ctx context.Context, organizationID, subjectID, requestingUserID string) ([]*domain.RoleBinding, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListForSubject(ctx, organizationID, subjectID)
}
