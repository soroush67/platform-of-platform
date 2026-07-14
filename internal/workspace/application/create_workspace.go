package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

const permissionWorkspaceManage = "workspace:manage"

// CreateWorkspaceInput implements
// `POST /api/v1/orgs/{org}/projects/{project}/workspaces`. Gated by
// workspace:manage, not organization:manage - unlike Project/Environment
// creation, this is exactly the day-to-day action the Write role was
// introduced to grant (internal/rbac/domain/role.go's own comment on
// why Write/Read finally diverge here).
type CreateWorkspaceInput struct {
	OrganizationID   string
	ProjectID        string
	RequestingUserID string
	Name             string
	ExecutionEngine  domain.ExecutionEngine
	EnvironmentID    *string
}

type CreateWorkspaceService struct {
	repo           WorkspaceRepository
	permChecker    PermissionChecker
	projectChecker ProjectChecker
}

func NewCreateWorkspaceService(repo WorkspaceRepository, permChecker PermissionChecker, projectChecker ProjectChecker) *CreateWorkspaceService {
	return &CreateWorkspaceService{repo: repo, permChecker: permChecker, projectChecker: projectChecker}
}

func (s *CreateWorkspaceService) Execute(ctx context.Context, in CreateWorkspaceInput) (*domain.Workspace, error) {
	exists, err := s.projectChecker.ProjectExists(ctx, in.OrganizationID, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionWorkspaceManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	// Known simplification, flagged rather than silently skipped: if
	// EnvironmentID is set, this doesn't verify it actually belongs to
	// ProjectID - Environment's own repository would need a lookup this
	// service doesn't do yet. Not required for this slice (there's no
	// promotion-flow feature depending on that invariant holding yet),
	// but a real gap once one does.
	ws, err := domain.NewWorkspace(in.OrganizationID, in.ProjectID, in.EnvironmentID, in.Name, in.ExecutionEngine)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, ws); err != nil {
		return nil, err
	}

	return ws, nil
}
