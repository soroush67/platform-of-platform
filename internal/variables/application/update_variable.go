package application

import (
	"context"

	"platform-of-platform/internal/variables/domain"
)

// UpdateVariableInput implements `PUT /orgs/{org}/variables/{variableID}` -
// only Value/Category/Sensitivity are updatable (see the postgres
// adapter's own comment on why Key/Scope stay immutable).
type UpdateVariableInput struct {
	OrganizationID   string
	RequestingUserID string
	VariableID       string
	Value            string
	Category         domain.Category
	Sensitivity      domain.Sensitivity
}

type UpdateVariableService struct {
	repo        VariableRepository
	membership  MembershipChecker
	permChecker PermissionChecker
}

func NewUpdateVariableService(repo VariableRepository, membership MembershipChecker, permChecker PermissionChecker) *UpdateVariableService {
	return &UpdateVariableService{repo: repo, membership: membership, permChecker: permChecker}
}

func (s *UpdateVariableService) Execute(ctx context.Context, in UpdateVariableInput) (*domain.Variable, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrVariableNotFound
	}

	v, err := s.repo.GetByID(ctx, in.OrganizationID, in.VariableID)
	if err != nil {
		return nil, err
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, requiredPermissionForScope(v.ScopeType))
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	if !in.Category.Valid() {
		return nil, &domain.ValidationError{Message: "category must be one of env_var, engine_var, file_template"}
	}
	if !in.Sensitivity.Valid() {
		return nil, &domain.ValidationError{Message: "sensitivity must be one of plain, sensitive"}
	}

	// UpdateVariableInput only ever carries a literal Value - there's no
	// secret_ref field to update here (docs/architecture/11-module-
	// secrets-state.md §2's own "no independent CRUD by design" - a
	// SecretRef-backed Variable can only be re-pointed by deleting and
	// recreating it). Clearing SecretRef keeps Value XOR SecretRef true
	// even when this call turns a secret-ref Variable into a plain one.
	v.Value = in.Value
	v.SecretRef = nil
	v.Category = in.Category
	v.Sensitivity = in.Sensitivity

	if err := s.repo.Update(ctx, v); err != nil {
		return nil, err
	}

	return v, nil
}
