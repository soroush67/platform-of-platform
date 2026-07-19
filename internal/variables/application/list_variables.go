package application

import (
	"context"

	"platform-of-platform/internal/variables/domain"
)

// ListVariablesService lists variables defined at exactly one scope
// (not the cascade - ResolveVariableService is that). Organization-
// scoped Variables stay membership-gated only (genuinely org-wide
// config); project/environment/workspace-scoped Variables are gated by
// the same per-Project visibility check (project_visibility.go) their
// owning Project now is - previously membership-gated only regardless
// of scope, same as every other read before this session's per-project
// visibility change.
type ListVariablesService struct {
	repo                VariableRepository
	membership          MembershipChecker
	permChecker         PermissionChecker
	visibilityChecker   VisibilityChecker
	workspaceChecker    WorkspaceChecker
	environmentResolver EnvironmentProjectResolver
}

func NewListVariablesService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker, visibilityChecker VisibilityChecker, workspaceChecker WorkspaceChecker, environmentResolver EnvironmentProjectResolver) *ListVariablesService {
	return &ListVariablesService{
		repo:                repo,
		membership:          membership,
		permChecker:         permChecker,
		visibilityChecker:   visibilityChecker,
		workspaceChecker:    workspaceChecker,
		environmentResolver: environmentResolver,
	}
}

func (s *ListVariablesService) Execute(ctx context.Context, organizationID, requestingUserID string, scopeType domain.ScopeType, scopeID string) ([]*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrScopeNotFound
	}

	var canAccess bool
	switch scopeType {
	case domain.ScopeTypeOrganization:
		canAccess = true
	case domain.ScopeTypeProject:
		canAccess, err = canAccessProject(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, scopeID)
	case domain.ScopeTypeWorkspace:
		var projectID string
		projectID, _, err = s.workspaceChecker.GetScope(ctx, organizationID, scopeID)
		if err == nil {
			canAccess, err = canAccessWorkspace(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, projectID, scopeID)
		}
	case domain.ScopeTypeEnvironment:
		var projectID string
		projectID, err = s.environmentResolver.ProjectIDForEnvironment(ctx, organizationID, scopeID)
		if err == nil {
			canAccess, err = canAccessProject(ctx, s.permChecker, s.visibilityChecker, organizationID, requestingUserID, projectID)
		}
	default:
		return nil, &domain.ValidationError{Message: "invalid scope_type: " + string(scopeType)}
	}
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, domain.ErrForbidden
	}

	return s.repo.ListByScope(ctx, organizationID, scopeType, scopeID)
}
