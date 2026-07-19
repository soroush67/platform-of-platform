package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// ListProjectsService implements `GET /api/v1/orgs/{org}/projects` -
// same gating as GetProjectService. Returns only the Projects this user
// can actually see (canAccessProject, project_visibility.go) - an Owner/
// Admin sees every Project same as before this change, everyone else
// sees only the ones they (or a Team they're in) have an explicit
// project-scope grant for. An empty result is a valid, non-error
// outcome - "you're a real org member, you just can't see any Project
// yet" is not the same as not being a member at all.
type ListProjectsService struct {
	repo              ProjectRepository
	membershipRepo    MembershipRepository
	permChecker       PermissionChecker
	visibilityChecker VisibilityChecker
}

func NewListProjectsService(repo ProjectRepository, membershipRepo MembershipRepository, permChecker PermissionChecker, visibilityChecker VisibilityChecker) *ListProjectsService {
	return &ListProjectsService{repo: repo, membershipRepo: membershipRepo, permChecker: permChecker, visibilityChecker: visibilityChecker}
}

func (s *ListProjectsService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]*domain.Project, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	projects, err := s.repo.ListByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// isAdmin checked once, outside the loop below - an Owner/Admin
	// bypasses per-Project visibility entirely (canAccessProject's own
	// reasoning), so there's no reason to re-check organization:manage
	// once per Project for the common "admin sees everything" case.
	isAdmin, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return projects, nil
	}

	visible := make([]*domain.Project, 0, len(projects))
	for _, p := range projects {
		canAccess, err := s.visibilityChecker.HasScopedPermission(ctx, organizationID, requestingUserID, permissionProjectRead, "project", p.ID)
		if err != nil {
			return nil, err
		}
		if canAccess {
			visible = append(visible, p)
		}
	}

	return visible, nil
}
