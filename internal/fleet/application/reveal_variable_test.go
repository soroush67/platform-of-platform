package application_test

import (
	"context"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

// fakeRevealVariableRepo is a configurable VariableRepository - the
// package's other fakeVariableRepo (deploy_executor_test.go) hardcodes
// GetByID to always return domain.ErrVariableNotFound, which doesn't
// fit RevealVariableService's own tests (GetByID is its one real read).
type fakeRevealVariableRepo struct {
	variables map[string]*domain.Variable
}

func newFakeRevealVariableRepo() *fakeRevealVariableRepo {
	return &fakeRevealVariableRepo{variables: map[string]*domain.Variable{}}
}
func (f *fakeRevealVariableRepo) Create(ctx context.Context, actorUserID string, v *domain.Variable) error {
	f.variables[v.ID] = v
	return nil
}
func (f *fakeRevealVariableRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Variable, error) {
	v, ok := f.variables[id]
	if !ok {
		return nil, domain.ErrVariableNotFound
	}
	return v, nil
}
func (f *fakeRevealVariableRepo) ListByComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Variable, error) {
	return nil, nil
}
func (f *fakeRevealVariableRepo) Update(ctx context.Context, actorUserID string, v *domain.Variable) error {
	return nil
}
func (f *fakeRevealVariableRepo) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	return nil
}

func TestRevealVariableService_SecretTyped_ResolvesLiveValue(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:read")
	repo := newFakeRevealVariableRepo()
	variable, _ := domain.NewVariableWithSecretRef("org-1", "cf-1", "DB_PASSWORD", "mount-1", "secret/data/fleet/compose-files/project-a/cf-1/DB_PASSWORD")
	repo.variables[variable.ID] = variable
	resolver := &fakeDeploySecretResolver{values: map[string]string{
		"mount-1|secret/data/fleet/compose-files/project-a/cf-1/DB_PASSWORD": "the-real-value",
	}}

	svc := application.NewRevealVariableService(repo, membership, permChecker, resolver)
	value, err := svc.Execute(context.Background(), "org-1", "user-1", variable.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "the-real-value" {
		t.Fatalf("expected the-real-value, got %q", value)
	}
}

func TestRevealVariableService_NonSecretVarType_ReturnsValidationError(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:read")
	repo := newFakeRevealVariableRepo()
	variable, _ := domain.NewVariable("org-1", "cf-1", "PLAIN_KEY", domain.VarTypeKV, "plain-value", "")
	repo.variables[variable.ID] = variable
	resolver := &fakeDeploySecretResolver{values: map[string]string{}}

	svc := application.NewRevealVariableService(repo, membership, permChecker, resolver)
	_, err := svc.Execute(context.Background(), "org-1", "user-1", variable.ID)
	if _, ok := err.(*domain.ValidationError); !ok {
		t.Fatalf("expected a *domain.ValidationError, got %T: %v", err, err)
	}
}

func TestRevealVariableService_PermissionDenied_ReturnsForbidden(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker() // compose_file:read not granted
	repo := newFakeRevealVariableRepo()
	resolver := &fakeDeploySecretResolver{values: map[string]string{}}

	svc := application.NewRevealVariableService(repo, membership, permChecker, resolver)
	_, err := svc.Execute(context.Background(), "org-1", "user-1", "some-variable-id")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}
