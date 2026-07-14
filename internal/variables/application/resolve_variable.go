package application

import (
	"context"
	"errors"

	"platform-of-platform/internal/variables/domain"
)

// ResolveVariableService implements the one piece of behavior this
// whole context exists for (docs/architecture/03-domain-model.md §7):
// "for a given Workspace, resolve a Variable key by checking Workspace-
// scoped first, then its Environment (if any), then its Project, then
// its Organization, taking the first match." Membership-gated only
// (any role) - resolving a variable to see its value is a read, same
// posture as every other read in this codebase.
type ResolveVariableService struct {
	repo             VariableRepository
	membership       MembershipChecker
	workspaceChecker WorkspaceChecker
}

func NewResolveVariableService(repo VariableRepository, membership MembershipChecker, workspaceChecker WorkspaceChecker) *ResolveVariableService {
	return &ResolveVariableService{repo: repo, membership: membership, workspaceChecker: workspaceChecker}
}

func (s *ResolveVariableService) Execute(ctx context.Context, organizationID, workspaceID, key, requestingUserID string) (*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrScopeNotFound
	}

	// Confirmed before calling GetScope so a nonexistent workspace maps
	// to variables' own domain.ErrScopeNotFound, not whatever error
	// Workspace's GetScope happens to return - this context never
	// inspects another context's error type (docs/architecture/
	// 18-backend-structure.md §3).
	exists, err := s.workspaceChecker.Exists(ctx, organizationID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrScopeNotFound
	}

	projectID, environmentID, err := s.workspaceChecker.GetScope(ctx, organizationID, workspaceID)
	if err != nil {
		return nil, err
	}

	// Walk domain.ScopeCascadeOrder's meaning explicitly rather than
	// looping over it generically - each level needs a different id
	// (workspaceID, *environmentID, projectID, organizationID), which a
	// generic loop over the enum values alone can't supply.
	if v, err := s.tryScope(ctx, organizationID, domain.ScopeTypeWorkspace, workspaceID, key); v != nil || err != nil {
		return v, err
	}
	if environmentID != nil {
		if v, err := s.tryScope(ctx, organizationID, domain.ScopeTypeEnvironment, *environmentID, key); v != nil || err != nil {
			return v, err
		}
	}
	if v, err := s.tryScope(ctx, organizationID, domain.ScopeTypeProject, projectID, key); v != nil || err != nil {
		return v, err
	}
	if v, err := s.tryScope(ctx, organizationID, domain.ScopeTypeOrganization, organizationID, key); v != nil || err != nil {
		return v, err
	}

	return nil, domain.ErrVariableNotFound
}

func (s *ResolveVariableService) tryScope(ctx context.Context, organizationID string, scopeType domain.ScopeType, scopeID, key string) (*domain.Variable, error) {
	v, err := s.repo.GetByScope(ctx, organizationID, scopeType, scopeID, key)
	if err != nil {
		if errors.Is(err, domain.ErrVariableNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}
