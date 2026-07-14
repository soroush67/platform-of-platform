package application

import (
	"context"

	"platform-of-platform/internal/variables/domain"
)

// DeleteVariableInput implements
// `DELETE /orgs/{org}/variables/{variableID}`.
type DeleteVariableInput struct {
	OrganizationID   string
	RequestingUserID string
	VariableID       string
}

type DeleteVariableService struct {
	repo        VariableRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDeleteVariableService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker) *DeleteVariableService {
	return &DeleteVariableService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *DeleteVariableService) Execute(ctx context.Context, in DeleteVariableInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrVariableNotFound
	}

	v, err := s.repo.GetByID(ctx, in.OrganizationID, in.VariableID)
	if err != nil {
		return err
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, requiredPermissionForScope(v.ScopeType))
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	return s.repo.Delete(ctx, in.OrganizationID, in.VariableID)
}
