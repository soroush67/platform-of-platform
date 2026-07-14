// Package domain holds the Variables context's pure Go types
// (docs/architecture/03-domain-model.md §7).
package domain

import (
	"errors"
	"fmt"
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

func (s ScopeType) valid() bool {
	switch s {
	case ScopeTypeOrganization, ScopeTypeProject, ScopeTypeEnvironment, ScopeTypeWorkspace:
		return true
	}
	return false
}

type Category string

const (
	CategoryEnvVar      Category = "env_var"
	CategoryEngineVar   Category = "engine_var"
	CategoryFileTemplate Category = "file_template"
)

func (c Category) valid() bool {
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

func (s Sensitivity) valid() bool {
	return s == SensitivityPlain || s == SensitivitySensitive
}

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Variable is the aggregate root (docs/architecture/03-domain-model.md
// §7). Value is always plain text in this codebase - see the migration's
// own comment on why secret_ref isn't supported yet (no Secrets context).
type Variable struct {
	ID             string
	OrganizationID string
	ScopeType      ScopeType
	ScopeID        string
	Key            string
	Category       Category
	Sensitivity    Sensitivity
	Value          string
	CreatedAt      time.Time
}

func NewVariable(organizationID string, scopeType ScopeType, scopeID, key string, category Category, sensitivity Sensitivity, value string) (*Variable, error) {
	if organizationID == "" || scopeID == "" {
		return nil, &ValidationError{Message: "organization_id and scope_id are required"}
	}
	if !scopeType.valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("scope_type %q must be one of organization, project, environment, workspace", scopeType)}
	}
	if !keyPattern.MatchString(key) {
		return nil, &ValidationError{Message: fmt.Sprintf("key %q must start with a letter/underscore and contain only letters, digits, or underscores", key)}
	}
	if !category.valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("category %q must be one of env_var, engine_var, file_template", category)}
	}
	if !sensitivity.valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("sensitivity %q must be one of plain, sensitive", sensitivity)}
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
