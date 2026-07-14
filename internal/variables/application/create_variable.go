package application

import (
	"context"

	"platform-of-platform/internal/variables/domain"
)

const (
	permissionOrganizationManage = "organization:manage"
	permissionWorkspaceManage    = "workspace:manage"
)

// CreateVariableInput implements `POST /api/v1/orgs/{org}/variables` -
// one generic endpoint for all four scope types (the request body
// carries scope_type/scope_id) rather than four separate URL routes,
// since the resource being created is the same shape regardless of
// scope - matches this codebase's existing preference for the smaller
// surface when it doesn't cost real clarity.
type CreateVariableInput struct {
	OrganizationID   string
	RequestingUserID string
	ScopeType        domain.ScopeType
	ScopeID          string
	Key              string
	Category         domain.Category
	Sensitivity      domain.Sensitivity
	Value            string
}

type CreateVariableService struct {
	repo               VariableRepository
	membership         MembershipChecker
	projectChecker     ProjectChecker
	environmentChecker EnvironmentChecker
	workspaceChecker   WorkspaceChecker
	permChecker        PermissionChecker
	orgChecker         OrganizationChecker
}

func NewCreateVariableService(repo VariableRepository, membership MembershipChecker, projectChecker ProjectChecker, environmentChecker EnvironmentChecker, workspaceChecker WorkspaceChecker, permChecker PermissionChecker, orgChecker OrganizationChecker) *CreateVariableService {
	return &CreateVariableService{
		repo: repo, membership: membership, projectChecker: projectChecker, environmentChecker: environmentChecker,
		workspaceChecker: workspaceChecker, permChecker: permChecker, orgChecker: orgChecker,
	}
}

func (s *CreateVariableService) Execute(ctx context.Context, in CreateVariableInput) (*domain.Variable, error) {
	// Validate the scope shape first (domain construction), then check
	// membership (a non-member gets "scope not found," not a 403 - same
	// ordering fix now applied to every Create*Service in this
	// codebase), then verify the scope_id actually resolves to something
	// real, then check permission.
	v, err := domain.NewVariable(in.OrganizationID, in.ScopeType, in.ScopeID, in.Key, in.Category, in.Sensitivity, in.Value)
	if err != nil {
		return nil, err
	}

	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrScopeNotFound
	}

	scopeExists, requiredPermission, err := s.resolveScope(ctx, in)
	if err != nil {
		return nil, err
	}
	if !scopeExists {
		return nil, domain.ErrScopeNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, requiredPermission)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	// An archived Organization can't grow new structure - same
	// enforcement point tenancy.CreateProjectService/workspace.
	// CreateWorkspaceService already apply.
	archived, err := s.orgChecker.IsArchived(ctx, in.OrganizationID)
	if err != nil {
		return nil, err
	}
	if archived {
		return nil, domain.ErrOrganizationArchived
	}

	if err := s.repo.Create(ctx, v); err != nil {
		return nil, err
	}

	return v, nil
}

// requiredPermissionForScope is the same tier-selection rule
// resolveScope below applies, factored out so UpdateVariableService/
// DeleteVariableService (which already have the Variable, via GetByID -
// there's no scope_id-existence check left to do, the row itself is the
// proof) can reuse just the permission-tier half.
func requiredPermissionForScope(scopeType domain.ScopeType) string {
	if scopeType == domain.ScopeTypeWorkspace {
		return permissionWorkspaceManage
	}
	return permissionOrganizationManage
}

// resolveScope checks the scope_id is real and picks the permission
// tier that gates it - organization/project/environment scopes are
// org-structural (organization:manage, the same tier Project/Environment
// creation already use); workspace scope is the day-to-day
// workspace:manage tier, matching Workspace creation's own gate.
func (s *CreateVariableService) resolveScope(ctx context.Context, in CreateVariableInput) (exists bool, requiredPermission string, err error) {
	switch in.ScopeType {
	case domain.ScopeTypeOrganization:
		return in.ScopeID == in.OrganizationID, permissionOrganizationManage, nil
	case domain.ScopeTypeProject:
		exists, err := s.projectChecker.ProjectExists(ctx, in.OrganizationID, in.ScopeID)
		return exists, permissionOrganizationManage, err
	case domain.ScopeTypeEnvironment:
		exists, err := s.environmentChecker.Exists(ctx, in.OrganizationID, in.ScopeID)
		return exists, permissionOrganizationManage, err
	case domain.ScopeTypeWorkspace:
		exists, err := s.workspaceChecker.Exists(ctx, in.OrganizationID, in.ScopeID)
		return exists, permissionWorkspaceManage, err
	default:
		// Unreachable: domain.NewVariable already rejected any other
		// scope_type before this method is ever called.
		return false, "", nil
	}
}
