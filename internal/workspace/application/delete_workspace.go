package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

const permissionWorkspaceDelete = "workspace:delete"

// DeleteWorkspaceRepository is the narrow extra port DeleteWorkspaceService
// needs beyond WorkspaceRepository's own Create/GetByID/ListByProject -
// same "Purge, not Delete" naming Tenancy's own DeleteProjectRepository
// uses, for the same reason: a genuine hard delete of the Workspace and
// everything scoped under it, not a soft state flip.
type DeleteWorkspaceRepository interface {
	Purge(ctx context.Context, organizationID, workspaceID string) error
}

// DeleteWorkspaceInput implements `DELETE /api/v1/orgs/{org}/projects/
// {project}/workspaces/{workspace}`. Gated by workspace:delete (Owner-
// only, stricter than workspace:manage - same organization:delete-style
// narrowing every other new *:delete permission uses), checked at this
// Workspace's own scope via ScopedPermissionChecker, matching
// CreateWorkspaceService's own HasPermissionAtScope call shape.
//
// Deliberately NO guard on Locked/an in-flight Run - operator's own
// explicit choice (confirmed via AskUserQuestion): always allow. A
// forcibly-deleted Workspace out from under a real `applying` Run
// leaves the Worker's own live process orphaned - WorkerReportService
// will find no matching row when it eventually reports back - a real,
// accepted risk, not an oversight.
type DeleteWorkspaceInput struct {
	OrganizationID   string
	ProjectID        string
	WorkspaceID      string
	RequestingUserID string
}

type DeleteWorkspaceService struct {
	repo           WorkspaceRepository
	deleteRepo     DeleteWorkspaceRepository
	membership     MembershipChecker
	permChecker    ScopedPermissionChecker
	projectChecker ProjectChecker
}

func NewDeleteWorkspaceService(repo WorkspaceRepository, deleteRepo DeleteWorkspaceRepository, membership MembershipChecker, permChecker ScopedPermissionChecker, projectChecker ProjectChecker) *DeleteWorkspaceService {
	return &DeleteWorkspaceService{repo: repo, deleteRepo: deleteRepo, membership: membership, permChecker: permChecker, projectChecker: projectChecker}
}

func (s *DeleteWorkspaceService) Execute(ctx context.Context, in DeleteWorkspaceInput) error {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrProjectNotFound
	}

	exists, err := s.projectChecker.ProjectExists(ctx, in.OrganizationID, in.ProjectID)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrProjectNotFound
	}

	allowed, err := s.permChecker.HasPermissionAtScope(ctx, in.OrganizationID, in.RequestingUserID, permissionWorkspaceDelete, &in.ProjectID, &in.WorkspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	ws, err := s.repo.GetByID(ctx, in.OrganizationID, in.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ProjectID != in.ProjectID {
		return domain.ErrWorkspaceNotFound
	}

	return s.deleteRepo.Purge(ctx, in.OrganizationID, in.WorkspaceID)
}
