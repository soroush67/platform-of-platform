package application

import (
	"context"
	"time"

	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
)

// CreateAPIKeyInput implements
// `POST /orgs/{org}/service-accounts/{sa}/api-keys`
// (docs/architecture/13-module-identity-rbac-tenancy.md §2). Gated by
// organization:manage, same as creating the ServiceAccount itself -
// issuing a credential is at least as sensitive as creating the
// identity it authenticates.
type CreateAPIKeyInput struct {
	OrganizationID   string
	RequestingUserID string
	ServiceAccountID string
	Name             string
	Scopes           []string
	ExpiresAt        *time.Time
}

// CreateAPIKeyResult carries the plaintext key alongside the persisted
// record - "shown once, never again, nothing server-side ever holds the
// plaintext" (docs/architecture/13-module-identity-rbac-tenancy.md §2's
// own wording, already the posture this session established for
// refresh/reset tokens).
type CreateAPIKeyResult struct {
	Key       *domain.APIKey
	Plaintext string
}

type CreateAPIKeyService struct {
	repo           APIKeyRepository
	saRepo         ServiceAccountRepository
	membership     MembershipChecker
	permChecker    PermissionChecker
	scopeValidator ScopeValidator
}

func NewCreateAPIKeyService(repo APIKeyRepository, saRepo ServiceAccountRepository, membership MembershipChecker, permChecker PermissionChecker, scopeValidator ScopeValidator) *CreateAPIKeyService {
	return &CreateAPIKeyService{repo: repo, saRepo: saRepo, membership: membership, permChecker: permChecker, scopeValidator: scopeValidator}
}

func (s *CreateAPIKeyService) Execute(ctx context.Context, in CreateAPIKeyInput) (*CreateAPIKeyResult, error) {
	isMember, err := s.membership.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrServiceAccountNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	// Confirm the ServiceAccount is real and belongs to this org before
	// issuing it a credential.
	if _, err := s.saRepo.GetByID(ctx, in.OrganizationID, in.ServiceAccountID); err != nil {
		return nil, err
	}

	for _, scope := range in.Scopes {
		if !s.scopeValidator.IsValidScope(scope) {
			return nil, &domain.ValidationError{Message: "unknown scope: " + scope}
		}
	}

	plaintext, err := auth.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}

	key, err := domain.NewAPIKey(domain.APIKeyOwnerTypeServiceAccount, in.ServiceAccountID, in.Name, auth.HashOpaqueToken(plaintext), in.Scopes, in.ExpiresAt)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, in.OrganizationID, key); err != nil {
		return nil, err
	}

	return &CreateAPIKeyResult{Key: key, Plaintext: plaintext}, nil
}
