// Package domain holds the Variables context's pure Go types
// (docs/architecture/03-domain-model.md §7).
package domain

import (
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var (
	ErrVariableNotFound = errors.New("variable not found")
	ErrForbidden        = errors.New("forbidden")
	// ErrScopeNotFound - the scope_id in the request doesn't genuinely
	// resolve to a real Organization/Project/Environment/Workspace in
	// this org, checked via the ScopeChecker ports below.
	ErrScopeNotFound = errors.New("scope not found")
	// ErrOrganizationArchived - same meaning as tenancy/domain's own
	// sentinel, redeclared here per this codebase's no-cross-context-
	// import rule. CreateVariableService checks this before creating a
	// new Variable in an archived Organization.
	ErrOrganizationArchived = errors.New("organization is archived")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// ScopeType is the closed set from docs/architecture/03-domain-model.md
// §7 - "for a given Workspace, resolve a Variable key by checking
// Workspace-scoped first, then its Environment, then its Project, then
// its Organization" - this exact ordering, weakest-to-broadest, is
// ScopeCascadeOrder below, the one piece of behavior this whole context
// exists to implement correctly.
type ScopeType string

const (
	ScopeTypeOrganization ScopeType = "organization"
	ScopeTypeProject      ScopeType = "project"
	ScopeTypeEnvironment  ScopeType = "environment"
	ScopeTypeWorkspace    ScopeType = "workspace"
)

// ScopeCascadeOrder - narrowest (most specific) first, matching Stage 3
// §7's resolution rule verbatim, and "identical precedence direction to
// compose-platform's Global-ComposeFile-vs-local-variable resolution
// already built and tested this session" per that same doc section -
// deliberately reused rather than inventing a new precedence model.
var ScopeCascadeOrder = []ScopeType{ScopeTypeWorkspace, ScopeTypeEnvironment, ScopeTypeProject, ScopeTypeOrganization}

func (s ScopeType) Valid() bool {
	switch s {
	case ScopeTypeOrganization, ScopeTypeProject, ScopeTypeEnvironment, ScopeTypeWorkspace:
		return true
	}
	return false
}

type Category string

const (
	CategoryEnvVar       Category = "env_var"
	CategoryEngineVar    Category = "engine_var"
	CategoryFileTemplate Category = "file_template"
)

func (c Category) Valid() bool {
	switch c {
	case CategoryEnvVar, CategoryEngineVar, CategoryFileTemplate:
		return true
	}
	return false
}

type Sensitivity string

const (
	SensitivityPlain     Sensitivity = "plain"
	SensitivitySensitive Sensitivity = "sensitive"
)

func (s Sensitivity) Valid() bool {
	return s == SensitivityPlain || s == SensitivitySensitive
}

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SecretReference is the value object from docs/architecture/11-module-
// secrets-state.md §2 - "no independent CRUD by design," it only ever
// appears embedded inside a Variable. MountID points at a real
// secrets/domain.SecretMount in this same Organization (validated by
// CreateVariableService's own SecretMountChecker port, not by this
// package - Variables never imports secrets/domain, per this
// codebase's no-cross-context-import rule). Path is the backend's own
// full path (e.g. Vault's "secret/data/database/prod/password") -
// this codebase doesn't standardize or rewrite it.
type SecretReference struct {
	MountID string
	Path    string
}

// Variable is the aggregate root (docs/architecture/03-domain-model.md
// §7). Value XOR SecretRef - exactly one is ever set, matching
// migrations/0018_secrets.up.sql's own CHECK constraint at the storage
// layer. A Variable backed by SecretRef never has its real value
// written anywhere in this codebase's own database - Value stays empty
// for it always; ResolveVariableService is what fetches the real
// content live, at resolve time, from the real backend.
type Variable struct {
	ID             string
	OrganizationID string
	ScopeType      ScopeType
	ScopeID        string
	Key            string
	Category       Category
	Sensitivity    Sensitivity
	Value          string
	SecretRef      *SecretReference
	CreatedAt      time.Time
}

// validateVariableFields is shared by NewVariable and
// NewVariableWithSecretRef - every field these two constructors have in
// common (everything except Value/SecretRef themselves, which are each
// constructor's own concern).
func validateVariableFields(organizationID string, scopeType ScopeType, scopeID, key string, category Category, sensitivity Sensitivity) error {
	if organizationID == "" || scopeID == "" {
		return &ValidationError{Message: "organization_id and scope_id are required"}
	}
	if !scopeType.Valid() {
		return &ValidationError{Message: "invalid scope_type: " + string(scopeType)}
	}
	if !keyPattern.MatchString(key) {
		return &ValidationError{Message: "key must match " + keyPattern.String()}
	}
	if !category.Valid() {
		return &ValidationError{Message: "invalid category: " + string(category)}
	}
	if !sensitivity.Valid() {
		return &ValidationError{Message: "invalid sensitivity: " + string(sensitivity)}
	}
	return nil
}

func NewVariable(organizationID string, scopeType ScopeType, scopeID, key string, category Category, sensitivity Sensitivity, value string) (*Variable, error) {
	if err := validateVariableFields(organizationID, scopeType, scopeID, key, category, sensitivity); err != nil {
		return nil, err
	}

	return &Variable{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		ScopeType:      scopeType,
		ScopeID:        scopeID,
		Key:            key,
		Category:       category,
		Sensitivity:    sensitivity,
		Value:          value,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// NewVariableWithSecretRef is NewVariable's counterpart for the
// SecretRef-backed path (docs/architecture/11-module-secrets-state.md
// §2) - mountID/path aren't validated for real existence here (Variables
// never imports secrets/domain to check a SecretMount actually exists;
// CreateVariableService's own SecretMountChecker port does that, same
// no-cross-context-import boundary SecretReference's own doc comment
// above already explains). This constructor only enforces that both are
// non-empty and leaves Value at its zero value permanently.
func NewVariableWithSecretRef(organizationID string, scopeType ScopeType, scopeID, key string, category Category, sensitivity Sensitivity, mountID, path string) (*Variable, error) {
	if err := validateVariableFields(organizationID, scopeType, scopeID, key, category, sensitivity); err != nil {
		return nil, err
	}
	if mountID == "" || path == "" {
		return nil, &ValidationError{Message: "secret_mount_id and secret_path are required"}
	}

	return &Variable{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		ScopeType:      scopeType,
		ScopeID:        scopeID,
		Key:            key,
		Category:       category,
		Sensitivity:    sensitivity,
		SecretRef:      &SecretReference{MountID: mountID, Path: path},
		CreatedAt:      time.Now().UTC(),
	}, nil
}
