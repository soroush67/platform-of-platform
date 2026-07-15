package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	// GetByUsername returns domain.ErrUserNotFound if no such user exists.
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
}

// RefreshTokenRepository is RefreshTokenService's own port - previously
// entirely unbuilt (access tokens had no companion refresh mechanism at
// all).
type RefreshTokenRepository interface {
	Create(ctx context.Context, t *domain.RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Revoke(ctx context.Context, id string) error
}

// PasswordResetTokenRepository is PasswordResetService's own port.
type PasswordResetTokenRepository interface {
	Create(ctx context.Context, t *domain.PasswordResetToken) error
	GetByHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error)
	MarkUsed(ctx context.Context, id string) error
}

// ServiceAccountRepository is CreateServiceAccountService's own port -
// previously entirely unbuilt (no ServiceAccount aggregate existed at
// all).
type ServiceAccountRepository interface {
	Create(ctx context.Context, sa *domain.ServiceAccount) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.ServiceAccount, error)
}

// APIKeyRepository is CreateAPIKeyService/RevokeAPIKeyService's own
// port.
type APIKeyRepository interface {
	Create(ctx context.Context, organizationID string, key *domain.APIKey) error
	GetByHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	TouchLastUsed(ctx context.Context, id string) error
	Revoke(ctx context.Context, organizationID, keyID string) error
}

// MembershipChecker/PermissionChecker - this context's own copies of
// the same port shape every other context declares locally
// (docs/architecture/18-backend-structure.md §3's dependency-inversion
// rule). Identity never needed these before - CreateUser/Login are
// unauthenticated by design - ServiceAccount/APIKey management is the
// first Identity action that needs an organization:manage gate.
type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// ScopeValidator is how CreateAPIKeyService validates a requested key's
// Scopes against RBAC's real, fixed Permission enum without Identity
// importing internal/rbac/domain directly - main.go wires a closure
// over rbac/domain.AllPermissions (see api_key.go's own comment on why).
type ScopeValidator interface {
	IsValidScope(scope string) bool
}

// ScopeValidatorFunc adapts a plain func to ScopeValidator - the same
// http.HandlerFunc-style adapter pattern, so main.go can pass a closure
// directly instead of defining a one-method struct just to satisfy the
// interface.
type ScopeValidatorFunc func(scope string) bool

func (f ScopeValidatorFunc) IsValidScope(scope string) bool { return f(scope) }
