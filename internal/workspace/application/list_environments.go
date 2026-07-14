package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

type ListEnvironmentsService struct {
	repo           EnvironmentRepository
	membership     MembershipChecker
	projectChecker ProjectChecker
}

func NewListEnvironmentsService(repo EnvironmentRepository, membership MembershipChecker, projectChecker ProjectChecker) *ListEnvironmentsService {
	return &ListEnvironmentsService{repo: repo, membership: membership, projectChecker: projectChecker}
}

func (s *ListEnvironmentsService) Execute(ctx context.Context, organizationID, projectID, requestingUserID string) ([]*domain.Environment, error) {
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

	return s.repo.ListByProject(ctx, organizationID, projectID)
}
