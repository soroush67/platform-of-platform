package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

type ListWorkspacesService struct {
	repo           WorkspaceRepository
	membership     MembershipChecker
	projectChecker ProjectChecker
}

func NewListWorkspacesService(repo WorkspaceRepository, membership MembershipChecker, projectChecker ProjectChecker) *ListWorkspacesService {
	return &ListWorkspacesService{repo: repo, membership: membership, projectChecker: projectChecker}
}

func (s *ListWorkspacesService) Execute(ctx context.Context, organizationID, projectID, requestingUserID string) ([]*domain.Workspace, error) {
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
