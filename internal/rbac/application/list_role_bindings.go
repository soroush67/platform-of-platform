package application

import (
	"context"
	"time"

	"platform-of-platform/internal/rbac/domain"
)

// RoleBindingSummary is a read-model composed from RBAC's own Role plus
// small cross-context lookups (Tenancy/Workspace/Identity) - same
// reasoning as Tenancy's own MemberSummary/TeamMemberSummary
// (internal/tenancy/application/list_members.go,
// list_team_members.go): every raw id from domain.RoleBinding stays
// present, RoleName/SubjectName/ScopeName are added alongside for
// display. An empty *Name is a real, displayable "couldn't resolve
// this" state (a since-deleted Team/Project/etc, or an
// organization-scope binding, which needs no lookup at all - the
// frontend already has the current org's own name loaded) - never a
// reason to fail the whole list.
type RoleBindingSummary struct {
	ID             string
	OrganizationID string
	RoleID         string
	RoleName       string
	SubjectType    string
	SubjectID      string
	SubjectName    string
	ScopeType      string
	ScopeID        string
	ScopeName      string
	Effect         string
	CreatedAt      time.Time
}

// ListRoleBindingsService implements
// `GET /orgs/{org}/role-bindings?subject_id=...` - "what can this
// subject do, and where" (docs/architecture/13-module-identity-rbac-
// tenancy.md §3). Empty subjectID lists every binding in the org. Now
// resolves display names alongside the raw ids (previously returned
// domain.RoleBinding directly, all-UUID) - the operator's own words:
// "I need to clearly see what I created."
type ListRoleBindingsService struct {
	repo                RoleBindingRepository
	membership          MembershipChecker
	roleRepo            RoleRepository
	userReader          UserReader
	teamNameReader      TeamNameReader
	serviceAccountNames ServiceAccountNameReader
	projectNames        ProjectNameReader
	workspaceNames      WorkspaceNameReader
}

func NewListRoleBindingsService(
	repo RoleBindingRepository,
	membership MembershipChecker,
	roleRepo RoleRepository,
	userReader UserReader,
	teamNameReader TeamNameReader,
	serviceAccountNames ServiceAccountNameReader,
	projectNames ProjectNameReader,
	workspaceNames WorkspaceNameReader,
) *ListRoleBindingsService {
	return &ListRoleBindingsService{
		repo:                repo,
		membership:          membership,
		roleRepo:            roleRepo,
		userReader:          userReader,
		teamNameReader:      teamNameReader,
		serviceAccountNames: serviceAccountNames,
		projectNames:        projectNames,
		workspaceNames:      workspaceNames,
	}
}

func (s *ListRoleBindingsService) Execute(ctx context.Context, organizationID, subjectID, requestingUserID string) ([]RoleBindingSummary, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	bindings, err := s.repo.ListForSubject(ctx, organizationID, subjectID)
	if err != nil {
		return nil, err
	}

	summaries := make([]RoleBindingSummary, 0, len(bindings))
	for _, b := range bindings {
		summary := RoleBindingSummary{
			ID: b.ID, OrganizationID: b.OrganizationID, RoleID: b.RoleID,
			SubjectType: b.SubjectType, SubjectID: b.SubjectID,
			ScopeType: b.ScopeType, ScopeID: b.ScopeID, Effect: b.Effect,
			CreatedAt: b.CreatedAt,
		}

		if role, err := s.roleRepo.GetByID(ctx, organizationID, b.RoleID); err == nil {
			summary.RoleName = role.Name
		}

		switch b.SubjectType {
		case domain.SubjectTypeUser:
			if username, _, found, err := s.userReader.GetUser(ctx, b.SubjectID); err == nil && found {
				summary.SubjectName = username
			}
		case domain.SubjectTypeTeam:
			if name, found, err := s.teamNameReader.GetTeamName(ctx, organizationID, b.SubjectID); err == nil && found {
				summary.SubjectName = name
			}
		case domain.SubjectTypeServiceAccount:
			if name, found, err := s.serviceAccountNames.GetServiceAccountName(ctx, organizationID, b.SubjectID); err == nil && found {
				summary.SubjectName = name
			}
		}

		switch b.ScopeType {
		case domain.ScopeTypeProject:
			if name, found, err := s.projectNames.GetProjectName(ctx, organizationID, b.ScopeID); err == nil && found {
				summary.ScopeName = name
			}
		case domain.ScopeTypeWorkspace:
			if name, found, err := s.workspaceNames.GetWorkspaceName(ctx, organizationID, b.ScopeID); err == nil && found {
				summary.ScopeName = name
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}
