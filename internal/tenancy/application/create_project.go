package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// CreateProjectInput implements `POST /api/v1/orgs/{org}/projects`
// (docs/architecture/04-api-design.md §1). Gated by organization:manage -
// the same permission that gates adding a member (add_member.go):
// creating a Project is an org-structural change, not a day-to-day
// action every "read" member should get, the same reasoning already
// applied there. No new Permission value was introduced for this -
// project:manage doesn't exist yet, deliberately: this is the first
// context where reusing organization:manage instead of minting a
// narrower permission was the honest call, since nothing in this
// codebase yet needs to distinguish "can manage org settings/members"
// from "can create projects."
type CreateProjectInput struct {
	OrganizationID   string
	RequestingUserID string
	Name             string
	Slug             string
	Description      string
}

type CreateProjectService struct {
	repo        ProjectRepository
	permChecker PermissionChecker
}

func NewCreateProjectService(repo ProjectRepository, permChecker PermissionChecker) *CreateProjectService {
	return &CreateProjectService{repo: repo, permChecker: permChecker}
}

func (s *CreateProjectService) Execute(ctx context.Context, in CreateProjectInput) (*domain.Project, error) {
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	project, err := domain.NewProject(in.OrganizationID, in.Name, in.Slug, in.Description)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, project); err != nil {
		return nil, err
	}

	return project, nil
}
