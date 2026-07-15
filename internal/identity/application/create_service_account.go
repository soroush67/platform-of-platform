package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

const permissionOrganizationManage = "organization:manage"

// CreateServiceAccountInput implements
// `POST /orgs/{org}/service-accounts` (docs/architecture/13-module-
// identity-rbac-tenancy.md §2). Gated by organization:manage - the same
// tier as creating a Team, another "org-structural, not day-to-day"
// action.
type CreateServiceAccountInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	Description      string
}

type CreateServiceAccountService struct {
	repo        ServiceAccountRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewCreateServiceAccountService(repo ServiceAccountRepository, membership MembershipChecker, permChecker PermissionChecker) *CreateServiceAccountService {
	return &CreateServiceAccountService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *CreateServiceAccountService) Execute(ctx context.Context, in CreateServiceAccountInput) (*domain.ServiceAccount, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrServiceAccountNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	sa, err := domain.NewServiceAccount(in.OrganizationID, in.Name, in.Description)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, sa); err != nil {
		return nil, err
	}

	return sa, nil
}
