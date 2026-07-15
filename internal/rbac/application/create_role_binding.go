package application

import (
	"context"

	"platform-of-platform/internal/rbac/domain"
)

// CreateRoleBindingInput implements `POST /orgs/{org}/role-bindings`
// (docs/architecture/13-module-identity-rbac-tenancy.md §3): { role_id,
// subject: {type, id}, scope: {type, id}, effect }. Effect defaults to
// "allow" when omitted - every binding created before EffectDeny
// existed was implicitly an allow, and every existing caller of this
// endpoint that doesn't yet know about "effect" should keep getting
// exactly that behavior.
type CreateRoleBindingInput struct {
	OrganizationID   string
	RequestingUserID string
	RoleID           string
	SubjectType      string
	SubjectID        string
	ScopeType        string
	ScopeID          string
	Effect           string
}

type CreateRoleBindingService struct {
	roleRepo            RoleRepository
	bindingRepo         RoleBindingRepository
	membership          MembershipChecker
	permChecker         PermissionChecker
	projectChecker      ProjectChecker
	workspaceChecker    WorkspaceChecker
	teamChecker         TeamChecker
	serviceAccountCheck ServiceAccountChecker
}

func NewCreateRoleBindingService(roleRepo RoleRepository, bindingRepo RoleBindingRepository, membership MembershipChecker, permChecker PermissionChecker, projectChecker ProjectChecker, workspaceChecker WorkspaceChecker, teamChecker TeamChecker, serviceAccountCheck ServiceAccountChecker) *CreateRoleBindingService {
	return &CreateRoleBindingService{
		roleRepo: roleRepo, bindingRepo: bindingRepo, membership: membership, permChecker: permChecker,
		projectChecker: projectChecker, workspaceChecker: workspaceChecker, teamChecker: teamChecker,
		serviceAccountCheck: serviceAccountCheck,
	}
}

func (s *CreateRoleBindingService) Execute(ctx context.Context, in CreateRoleBindingInput) (*domain.RoleBinding, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	if !domain.ValidSubjectTypes[in.SubjectType] {
		return nil, &domain.ValidationError{Message: "subject.type must be one of: user, team, service_account"}
	}
	if !domain.ValidScopeTypes[in.ScopeType] {
		return nil, &domain.ValidationError{Message: "scope.type must be one of: organization, project, workspace"}
	}
	if in.Effect == "" {
		in.Effect = domain.EffectAllow
	}
	if !domain.ValidEffects[in.Effect] {
		return nil, &domain.ValidationError{Message: "effect must be one of: allow, deny"}
	}

	// docs/architecture/03-domain-model.md §4's Invariant: "a
	// RoleBinding's scope must be a resource within the same
	// Organization as the Role it references" - a custom Role from
	// this org, or any built-in (organization_id nil, visible
	// everywhere per roles_isolation's own RLS policy).
	role, err := s.roleRepo.GetByID(ctx, in.OrganizationID, in.RoleID)
	if err != nil {
		return nil, err
	}
	if role.OrganizationID != nil && *role.OrganizationID != in.OrganizationID {
		return nil, domain.ErrRoleNotFound
	}

	switch in.SubjectType {
	case domain.SubjectTypeUser:
		subjectIsMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.SubjectID)
		if err != nil {
			return nil, err
		}
		if !subjectIsMember {
			return nil, &domain.ValidationError{Message: "subject.id is not a member of this organization"}
		}
	case domain.SubjectTypeTeam:
		exists, err := s.teamChecker.TeamExists(ctx, in.OrganizationID, in.SubjectID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "subject.id is not a team in this organization"}
		}
	case domain.SubjectTypeServiceAccount:
		exists, err := s.serviceAccountCheck.ServiceAccountExists(ctx, in.OrganizationID, in.SubjectID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "subject.id is not a service account in this organization"}
		}
	}

	switch in.ScopeType {
	case domain.ScopeTypeOrganization:
		if in.ScopeID != in.OrganizationID {
			return nil, &domain.ValidationError{Message: "scope.id must be this organization's id for scope.type organization"}
		}
	case domain.ScopeTypeProject:
		exists, err := s.projectChecker.ProjectExists(ctx, in.OrganizationID, in.ScopeID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "scope.id is not a project in this organization"}
		}
	case domain.ScopeTypeWorkspace:
		exists, err := s.workspaceChecker.Exists(ctx, in.OrganizationID, in.ScopeID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "scope.id is not a workspace in this organization"}
		}
	}

	binding := domain.NewRoleBinding(in.OrganizationID, in.RoleID, in.SubjectType, in.SubjectID, in.ScopeType, in.ScopeID, in.Effect)

	if err := s.bindingRepo.Create(ctx, binding); err != nil {
		return nil, err
	}

	return binding, nil
}
