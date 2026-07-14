package application

import (
	"context"

	"platform-of-platform/internal/workspace/domain"
)

const permissionOrganizationManage = "organization:manage"

// CreateEnvironmentInput implements
// `POST /api/v1/orgs/{org}/projects/{project}/environments`. Gated by
// organization:manage, same as Project creation
// (tenancy/application/create_project.go) - defining the promotion
// pipeline's stages is an org-structural decision, not opened up to the
// Write role the way workspace:manage is (see create_workspace.go's own
// comment on why that one's different).
type CreateEnvironmentInput struct {
	OrganizationID   string
	ProjectID        string
	RequestingUserID string
	Name             string
	PromotionRank    int
	RequiresApproval bool
}

type CreateEnvironmentService struct {
	repo           EnvironmentRepository
	membership     MembershipChecker
	permChecker    PermissionChecker
	projectChecker ProjectChecker
}

func NewCreateEnvironmentService(repo EnvironmentRepository, membership MembershipChecker, permChecker PermissionChecker, projectChecker ProjectChecker) *CreateEnvironmentService {
	return &CreateEnvironmentService{repo: repo, membership: membership, permChecker: permChecker, projectChecker: projectChecker}
}

func (s *CreateEnvironmentService) Execute(ctx context.Context, in CreateEnvironmentInput) (*domain.Environment, error) {
	// See CreateWorkspaceService's identical check for why membership is
	// verified before anything else - a non-member gets "project not
	// found," not a 403 that would confirm the project/org are real.
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

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	env, err := domain.NewEnvironment(in.OrganizationID, in.ProjectID, in.Name, in.PromotionRank, in.RequiresApproval)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, env); err != nil {
		return nil, err
	}

	return env, nil
}
