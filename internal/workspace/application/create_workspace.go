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
	repo            WorkspaceRepository
	environmentRepo EnvironmentRepository
	membership      MembershipChecker
	permChecker     ScopedPermissionChecker
	projectChecker  ProjectChecker
}

func NewCreateWorkspaceService(repo WorkspaceRepository, environmentRepo EnvironmentRepository, membership MembershipChecker, permChecker ScopedPermissionChecker, projectChecker ProjectChecker) *CreateWorkspaceService {
	return &CreateWorkspaceService{repo: repo, environmentRepo: environmentRepo, membership: membership, permChecker: permChecker, projectChecker: projectChecker}
}

func (s *CreateWorkspaceService) Execute(ctx context.Context, in CreateWorkspaceInput) (*domain.Workspace, error) {
	// Membership checked before anything else, so a non-member gets the
	// same "not found" response a genuinely nonexistent project would
	// give - not the 403 that would confirm the project (and therefore
	// the org) is real. Previously this service jumped straight to the
	// permission check, which - since HasPermission requires a role
	// binding, i.e. membership, to ever return true - already blocked a
	// non-member's write, just with the wrong status code.
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrProjectNotFound
	}

	exists, err := s.projectChecker.ProjectExists(ctx, in.OrganizationID, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrProjectNotFound
	}

	allowed, err := s.permChecker.HasPermissionAtScope(ctx, in.OrganizationID, in.RequestingUserID, permissionWorkspaceManage, &in.ProjectID, nil)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	// Verify EnvironmentID, if given, actually belongs to ProjectID -
	// previously a documented, unenforced gap; Environment lives in this
	// same bounded context (no cross-context port needed, just a direct
	// repository call, unlike ProjectChecker which genuinely crosses
	// into Tenancy).
	if in.EnvironmentID != nil {
		env, err := s.environmentRepo.GetByID(ctx, in.OrganizationID, *in.EnvironmentID)
		if err != nil {
			if err == domain.ErrEnvironmentNotFound {
				return nil, &domain.ValidationError{Message: "environment_id does not exist"}
			}
			return nil, err
		}
		if env.ProjectID != in.ProjectID {
			return nil, &domain.ValidationError{Message: "environment_id does not belong to this project"}
		}
	}

	ws, err := domain.NewWorkspace(in.OrganizationID, in.ProjectID, in.EnvironmentID, in.Name, in.ExecutionEngine)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, ws); err != nil {
		return nil, err
	}

	return ws, nil
}
