package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

// GetEnvironmentService - membership-gated only (any role), same
// reasoning as GetProjectService in the Tenancy context: reading isn't
// an org-structural action.
type GetEnvironmentService struct {
	repo           EnvironmentRepository
	membership     MembershipChecker
	projectChecker ProjectChecker
}

func NewGetEnvironmentService(repo EnvironmentRepository, membership MembershipChecker, projectChecker ProjectChecker) *GetEnvironmentService {
	return &GetEnvironmentService{repo: repo, membership: membership, projectChecker: projectChecker}
}

func (s *GetEnvironmentService) Execute(ctx context.Context, organizationID, projectID, environmentID, requestingUserID string) (*domain.Environment, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrProjectNotFound
	}

	exists, err := s.projectChecker.ProjectExists(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	env, err := s.repo.GetByID(ctx, organizationID, environmentID)
	if err != nil {
		return nil, err
	}
	if env.ProjectID != projectID {
		// Same "don't leak a real row through the wrong parent's URL"
		// posture already proven for cross-org Project isolation - here
		// it's cross-*project*, same org.
		return nil, domain.ErrEnvironmentNotFound
	}

	return env, nil
}
