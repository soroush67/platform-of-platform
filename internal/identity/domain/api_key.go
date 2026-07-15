package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrAPIKeyInvalid = errors.New("api key is invalid, expired, or revoked")

const (
	APIKeyOwnerTypeUser           = "user"
	APIKeyOwnerTypeServiceAccount = "service_account"
)

// DefaultAPIKeyTTL - an API key with no expires_at in the request isn't
// given an indefinite lifetime (docs/architecture/13-module-identity-
// rbac-tenancy.md §2's own "shown once... never again" posture already
// implies these are meant to be rotated, not permanent) - one year is a
// real, bounded default, not "forever."
const DefaultAPIKeyTTL = 365 * 24 * time.Hour

// APIKey is an entity, not owned by RBAC - "owned by either a User or a
// ServiceAccount" (docs/architecture/03-domain-model.md §3). Only the
// ServiceAccount-owned path is actually issued by this codebase today
// (CreateAPIKeyService) - a User-owned key is schema-legal
// (migrations/0017's own CHECK constraint) but nothing constructs one
// yet, a named, narrower-than-the-doc scope.
//
// Scopes (docs/architecture/13-module-identity-rbac-tenancy.md §2:
// "optional narrowing below the owner's own RBAC grants") are validated
// against RBAC's own fixed Permission enum via a ScopeValidator port
// CreateAPIKeyService takes (internal/identity/application/ports.go) -
// Identity can't import internal/rbac/domain directly (this codebase's
// own no-cross-context-import rule), so main.go wires a closure over
// rbac/domain.AllPermissions instead, the same dependency-inversion
// shape every other cross-context check in this codebase already uses.
// This type just carries the validated result. Real,
// named gap: nothing in the request-authorization path currently
// *intersects* a resolved API key's own Scopes with what RBAC would
// otherwise grant its owner - a key's Scopes are recorded and
// returned via the API, but don't yet narrow what a request
// authenticated with that key can actually do beyond what the owning
// ServiceAccount's RoleBindings already allow. Closing that fully
// would mean threading an optional scope-restriction through every
// application service's Execute() call, a much larger, separate change
// not folded into this pass.
type APIKey struct {
	ID         string
	OwnerType  string
	OwnerID    string
	Name       string
	KeyHash    string
	Scopes     []string
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

func NewAPIKey(ownerType, ownerID, name, keyHash string, scopes []string, expiresAt *time.Time) (*APIKey, error) {
	if ownerType != APIKeyOwnerTypeUser && ownerType != APIKeyOwnerTypeServiceAccount {
		return nil, &ValidationError{Message: "owner_type must be one of: user, service_account"}
	}
	if ownerID == "" {
		return nil, &ValidationError{Message: "owner_id is required"}
	}
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}

	expiry := time.Now().UTC().Add(DefaultAPIKeyTTL)
	if expiresAt != nil {
		if !expiresAt.After(time.Now().UTC()) {
			return nil, &ValidationError{Message: "expires_at must be in the future"}
		}
		expiry = *expiresAt
	}

	return &APIKey{
		ID:        uuid.NewString(),
		OwnerType: ownerType,
		OwnerID:   ownerID,
		Name:      name,
		KeyHash:   keyHash,
		Scopes:    scopes,
		ExpiresAt: expiry,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Valid mirrors RefreshToken/PasswordResetToken's own real, testable
// domain method rather than inlining the same two comparisons at every
// call site.
func (k *APIKey) Valid() bool {
	return k.RevokedAt == nil && time.Now().UTC().Before(k.ExpiresAt)
}
