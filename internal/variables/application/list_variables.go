package application

import (
	"context"

	"platform-of-platform/internal/variables/domain"
)

// ListVariablesService lists variables defined at exactly one scope
// (not the cascade - ResolveVariableService is that) - membership-gated
// only, same as every other read.
type ListVariablesService struct {
	repo       VariableRepository
	membership MembershipChecker
}

func NewListVariablesService(repo VariableRepository, membership MembershipChecker) *ListVariablesService {
	return &ListVariablesService{repo: repo, membership: membership}
}

func (s *ListVariablesService) Execute(ctx context.Context, organizationID, requestingUserID string, scopeType domain.ScopeType, scopeID string) ([]*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrScopeNotFound
	}

	return s.repo.ListByScope(ctx, organizationID, scopeType, scopeID)
}
