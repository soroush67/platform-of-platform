package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

type CreateVariableInput struct {
	OrganizationID   string
	RequestingUserID string
	ComposeFileID    string
	Key              string
	VarType          string
	Value            string
	SecretMountID    string
	SecretPath       string
	FileTargetPath   string
}

type CreateVariableService struct {
	repo        VariableRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	mountCheck  SecretMountChecker
}

func NewCreateVariableService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker) *CreateVariableService {
	return &CreateVariableService{repo: repo, membership: membership, permChecker: permChecker, mountCheck: mountCheck}
}

func (s *CreateVariableService) Execute(ctx context.Context, in CreateVariableInput) (*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionComposeFileManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	var variable *domain.Variable
	if domain.VarType(in.VarType) == domain.VarTypeSecret {
		exists, err := s.mountCheck.SecretMountExists(ctx, in.OrganizationID, in.SecretMountID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &domain.ValidationError{Message: "secret_mount_id does not reference a real secret mount in this organization"}
		}
		variable, err = domain.NewVariableWithSecretRef(in.OrganizationID, in.ComposeFileID, in.Key, in.SecretMountID, in.SecretPath)
		if err != nil {
			return nil, err
		}
	} else {
		variable, err = domain.NewVariable(in.OrganizationID, in.ComposeFileID, in.Key, domain.VarType(in.VarType), in.Value, in.FileTargetPath)
		if err != nil {
			return nil, err
		}
	}

	if err := s.repo.Create(ctx, in.RequestingUserID, variable); err != nil {
		return nil, err
	}
	return variable, nil
}

type ListVariablesService struct {
	repo        VariableRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewListVariablesService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker) *ListVariablesService {
	return &ListVariablesService{repo: repo, membership: membership, permChecker: permChecker}
}

// Execute masks secret-typed values the same way the ported Python
// original's own _variable_out did - never resolves the real value for
// this listing path (that only ever happens live, in DeployExecutor).
func (s *ListVariablesService) Execute(ctx context.Context, organizationID, requestingUserID, composeFileID string) ([]*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileRead)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}
	variables, err := s.repo.ListByComposeFile(ctx, organizationID, composeFileID)
	if err != nil {
		return nil, err
	}
	for _, v := range variables {
		if v.VarType == domain.VarTypeSecret {
			v.Value = MaskedValue
		}
	}
	return variables, nil
}

// RevealVariableService live-resolves a secret-typed Variable's real
// value from Vault - Fleet's own equivalent of the sibling
// internal/variables context's already-existing ResolveVariableService,
// previously missing here entirely (a secret-typed fleet_variables row
// could be created/referenced but never viewed again through this
// context). Never persisted anywhere, matching every other resolve path
// in this codebase.
type RevealVariableService struct {
	repo           VariableRepository
	membership     MembershipChecker
	permChecker    PermissionChecker
	secretResolver SecretResolver
}

func NewRevealVariableService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker, secretResolver SecretResolver) *RevealVariableService {
	return &RevealVariableService{repo: repo, membership: membership, permChecker: permChecker, secretResolver: secretResolver}
}

func (s *RevealVariableService) Execute(ctx context.Context, organizationID, requestingUserID, variableID string) (string, error) {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return "", err
	}
	if !isMember {
		return "", domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileRead)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", domain.ErrForbidden
	}

	variable, err := s.repo.GetByID(ctx, organizationID, variableID)
	if err != nil {
		return "", err
	}
	if variable.VarType != domain.VarTypeSecret || variable.SecretRef == nil {
		return "", &domain.ValidationError{Message: "only secret-typed variables can be revealed"}
	}

	return s.secretResolver.ResolveValue(ctx, organizationID, variable.SecretRef.MountID, variable.SecretRef.Path)
}

type UpdateVariableInput struct {
	OrganizationID   string
	RequestingUserID string
	VariableID       string
	Value            *string
	SecretMountID    *string
	SecretPath       *string
	FileTargetPath   *string
}

type UpdateVariableService struct {
	repo        VariableRepository
	membership  MembershipChecker
	permChecker PermissionChecker
	mountCheck  SecretMountChecker
}

func NewUpdateVariableService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker, mountCheck SecretMountChecker) *UpdateVariableService {
	return &UpdateVariableService{repo: repo, membership: membership, permChecker: permChecker, mountCheck: mountCheck}
}

func (s *UpdateVariableService) Execute(ctx context.Context, in UpdateVariableInput) (*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionComposeFileManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	variable, err := s.repo.GetByID(ctx, in.OrganizationID, in.VariableID)
	if err != nil {
		return nil, err
	}

	if variable.VarType == domain.VarTypeSecret {
		mountID := variable.SecretRef.MountID
		path := variable.SecretRef.Path
		if in.SecretMountID != nil {
			mountID = *in.SecretMountID
		}
		if in.SecretPath != nil {
			path = *in.SecretPath
		}
		if in.SecretMountID != nil {
			exists, err := s.mountCheck.SecretMountExists(ctx, in.OrganizationID, mountID)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, &domain.ValidationError{Message: "secret_mount_id does not reference a real secret mount in this organization"}
			}
		}
		variable.SecretRef = &domain.SecretReference{MountID: mountID, Path: path}
	} else if in.Value != nil {
		variable.Value = *in.Value
	}
	if in.FileTargetPath != nil {
		if variable.VarType.RequiresFileTargetPath() && *in.FileTargetPath == "" {
			return nil, &domain.ValidationError{Message: "file_target_path is required for var_type " + string(variable.VarType)}
		}
		variable.FileTargetPath = *in.FileTargetPath
	}

	if err := s.repo.Update(ctx, in.RequestingUserID, variable); err != nil {
		return nil, err
	}
	return variable, nil
}

type DeleteVariableService struct {
	repo        VariableRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewDeleteVariableService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker) *DeleteVariableService {
	return &DeleteVariableService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *DeleteVariableService) Execute(ctx context.Context, organizationID, requestingUserID, variableID string) error {
	isMember, err := s.membership.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return domain.ErrForbidden
	}
	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionComposeFileManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	return s.repo.Delete(ctx, requestingUserID, organizationID, variableID)
}
