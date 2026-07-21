package domain_test

import (
	"errors"
	"testing"

	"platform-of-platform/internal/workspace/domain"
)

func TestNewWorkspace_RejectsComposeEngine(t *testing.T) {
	_, err := domain.NewWorkspace("org-1", "project-1", nil, "ws", domain.ExecutionEngineCompose)
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a validation error rejecting the retired compose engine, got: %v", err)
	}
}

func TestNewWorkspace_AcceptsEveryRemainingEngine(t *testing.T) {
	for _, engine := range []domain.ExecutionEngine{
		domain.ExecutionEngineTerraform, domain.ExecutionEngineOpenTofu, domain.ExecutionEngineAnsible,
		domain.ExecutionEngineHelm, domain.ExecutionEnginePacker, domain.ExecutionEngineKubespray, domain.ExecutionEngineK8s,
	} {
		if _, err := domain.NewWorkspace("org-1", "project-1", nil, "ws", engine); err != nil {
			t.Errorf("expected engine %q to still be accepted, got: %v", engine, err)
		}
	}
}
