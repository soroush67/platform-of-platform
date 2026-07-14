package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrWorkspaceNotFound = errors.New("workspace not found")

// ExecutionEngine is a closed set (docs/architecture/03-domain-model.md
// §5) - same "closed set -> real Go type" reasoning as
// identity/domain.AuthSource.
type ExecutionEngine string

const (
	ExecutionEngineTerraform ExecutionEngine = "terraform"
	ExecutionEngineOpenTofu  ExecutionEngine = "opentofu"
	ExecutionEngineAnsible   ExecutionEngine = "ansible"
	ExecutionEngineHelm      ExecutionEngine = "helm"
	ExecutionEngineCompose   ExecutionEngine = "compose"
	ExecutionEnginePacker    ExecutionEngine = "packer"
	ExecutionEngineKubespray ExecutionEngine = "kubespray"
	ExecutionEngineK8s       ExecutionEngine = "kubernetes"
)

func (e ExecutionEngine) valid() bool {
	switch e {
	case ExecutionEngineTerraform, ExecutionEngineOpenTofu, ExecutionEngineAnsible, ExecutionEngineHelm,
		ExecutionEngineCompose, ExecutionEnginePacker, ExecutionEngineKubespray, ExecutionEngineK8s:
		return true
	}
	return false
}

// Workspace is the aggregate root "the product actually revolves
// around" (Stage 3 context map) - references ProjectID and, optionally,
// EnvironmentID (docs/architecture/03-domain-model.md §5).
//
// Invariant this type deliberately can't violate: ExecutionEngine is
// immutable after the first successful Run (Stage 3 §5) - there is no
// SetExecutionEngine method, and no update path at all yet (the
// Execution context that would make a workspace's *first* Run a real
// event doesn't exist in this codebase yet either) - so the invariant
// holds trivially for now, by omission, not by an enforced check against
// real Run history.
type Workspace struct {
	ID                    string
	OrganizationID        string
	ProjectID             string
	EnvironmentID         *string
	Name                  string
	ExecutionEngine       ExecutionEngine
	VCSLinkID             *string
	CurrentStateVersionID *string
	Locked                bool
	LockedByRunID         *string
	CreatedAt             time.Time
}

func NewWorkspace(organizationID, projectID string, environmentID *string, name string, engine ExecutionEngine) (*Workspace, error) {
	if organizationID == "" || projectID == "" {
		return nil, &ValidationError{Message: "organization_id and project_id are required"}
	}
	if !namePattern.MatchString(name) {
		return nil, &ValidationError{Message: "name must start with a letter/digit and contain only letters, digits, - or _"}
	}
	if !engine.valid() {
		return nil, &ValidationError{Message: "execution_engine must be one of terraform, opentofu, ansible, helm, compose, packer, kubespray, kubernetes"}
	}

	return &Workspace{
		ID:              uuid.NewString(),
		OrganizationID:  organizationID,
		ProjectID:       projectID,
		EnvironmentID:   environmentID,
		Name:            name,
		ExecutionEngine: engine,
		Locked:          false,
		CreatedAt:       time.Now().UTC(),
	}, nil
}
