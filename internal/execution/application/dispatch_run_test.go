package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
	"platform-of-platform/internal/platform/outbox"
)

func runQueuedEvent(runID, workspaceID string) outbox.Event {
	payload, _ := json.Marshal(map[string]string{"target_id": runID, "workspace_id": workspaceID})
	return outbox.Event{OrganizationID: testOrgID, EventType: "RunQueued", Payload: payload}
}

func TestRunDispatchService_IgnoresOtherEventTypes(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	svc := application.NewRunDispatchService(runRepo, newFakeWorkspaceEngineReader(), newFakeVariableResolver(), newFakeWorkerDispatcher(true), locker)

	err := svc.HandleEvent(context.Background(), outbox.Event{EventType: "SomeOtherEvent"})
	if err != nil {
		t.Fatalf("expected a non-RunQueued event to be a silent no-op, got: %v", err)
	}
}

func TestRunDispatchService_RedeliveredEventIsANoOp(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	_ = run.Cancel() // not queued anymore - simulates an already-dispatched (or canceled) run
	runRepo.put(run)
	dispatcher := newFakeWorkerDispatcher(true)
	svc := application.NewRunDispatchService(runRepo, newFakeWorkspaceEngineReader(), newFakeVariableResolver(), dispatcher, locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("expected a redelivered event for a non-queued run to be a silent no-op, got: %v", err)
	}
	if dispatcher.calls != 0 {
		t.Errorf("expected the worker dispatcher not to be called for an already-claimed run, got %d calls", dispatcher.calls)
	}
}

func TestRunDispatchService_MissingConfigFailsTheRun(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "compose")
	// Deliberately no variable set on the resolver.
	svc := application.NewRunDispatchService(runRepo, engineReader, newFakeVariableResolver(), newFakeWorkerDispatcher(true), locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("expected missing config to be handled by failing the run, not returning an error, got: %v", err)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusFailed {
		t.Errorf("expected the run to be marked failed, got %q", got.Status)
	}
	if locker.isLocked(testWorkspaceID) {
		t.Error("expected the workspace lock to be released when the run fails")
	}
}

func TestRunDispatchService_NoWorkerAvailableRevertsToQueued(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "compose")
	resolver := newFakeVariableResolver()
	resolver.set(testOrgID, testWorkspaceID, "compose_file", "version: '3'")
	svc := application.NewRunDispatchService(runRepo, engineReader, resolver, newFakeWorkerDispatcher(false), locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if !errors.Is(err, domain.ErrNoWorkerAvailable) {
		t.Fatalf("expected ErrNoWorkerAvailable, got: %v", err)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusQueued {
		t.Errorf("expected the run to be reverted to queued, got %q", got.Status)
	}
	if !locker.isLocked(testWorkspaceID) {
		t.Error("expected the workspace lock to still be held so a later redelivery can retry")
	}
}

// TestRunDispatchService_TerraformEngineResolvesTerraformConfigVariable
// proves configVariableKeyByEngine's own per-engine branch actually
// picks "terraform_config", not the "compose_file" key a terraform
// Workspace's Variables would never contain.
func TestRunDispatchService_TerraformEngineResolvesTerraformConfigVariable(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "terraform")
	resolver := newFakeVariableResolver()
	resolver.set(testOrgID, testWorkspaceID, "terraform_config", `resource "local_file" "x" { filename = "x" content = "y" }`)
	dispatcher := newFakeWorkerDispatcher(true)
	svc := application.NewRunDispatchService(runRepo, engineReader, resolver, dispatcher, locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if dispatcher.calls != 1 {
		t.Errorf("expected the dispatcher to be called once, got %d", dispatcher.calls)
	}
	if dispatcher.lastConfigBundle == "" {
		t.Error("expected the terraform_config variable's content to be dispatched as the config bundle")
	}
}

// TestRunDispatchService_OpenTofuEngineResolvesOpenTofuConfigVariable
// mirrors the terraform case above - proves the opentofu branch of
// configVariableKeyByEngine resolves its own distinct key.
func TestRunDispatchService_OpenTofuEngineResolvesOpenTofuConfigVariable(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "opentofu")
	resolver := newFakeVariableResolver()
	resolver.set(testOrgID, testWorkspaceID, "opentofu_config", `resource "local_file" "x" { filename = "x" content = "y" }`)
	dispatcher := newFakeWorkerDispatcher(true)
	svc := application.NewRunDispatchService(runRepo, engineReader, resolver, dispatcher, locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if dispatcher.lastConfigBundle == "" {
		t.Error("expected the opentofu_config variable's content to be dispatched as the config bundle")
	}
}

// TestRunDispatchService_AnsibleEngineResolvesAnsiblePlaybookVariable
// mirrors the terraform case above for the ansible branch.
func TestRunDispatchService_AnsibleEngineResolvesAnsiblePlaybookVariable(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "ansible")
	resolver := newFakeVariableResolver()
	resolver.set(testOrgID, testWorkspaceID, "ansible_playbook", `- hosts: localhost\n  tasks: []`)
	dispatcher := newFakeWorkerDispatcher(true)
	svc := application.NewRunDispatchService(runRepo, engineReader, resolver, dispatcher, locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if dispatcher.lastConfigBundle == "" {
		t.Error("expected the ansible_playbook variable's content to be dispatched as the config bundle")
	}
}

// TestRunDispatchService_PackerEngineResolvesPackerTemplateVariable
// mirrors the terraform case above for the packer branch.
func TestRunDispatchService_PackerEngineResolvesPackerTemplateVariable(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "packer")
	resolver := newFakeVariableResolver()
	resolver.set(testOrgID, testWorkspaceID, "packer_template", `source "docker" "x" {}`)
	dispatcher := newFakeWorkerDispatcher(true)
	svc := application.NewRunDispatchService(runRepo, engineReader, resolver, dispatcher, locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if dispatcher.lastConfigBundle == "" {
		t.Error("expected the packer_template variable's content to be dispatched as the config bundle")
	}
}

// TestRunDispatchService_EngineWithNoConfigKeyMappingFailsTheRun covers
// a real ExecutionEngine enum value (Workspace creation already
// accepts it) that has no configVariableKeyByEngine entry yet - the
// remaining three engines this Worker doesn't implement.
func TestRunDispatchService_EngineWithNoConfigKeyMappingFailsTheRun(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "helm")
	svc := application.NewRunDispatchService(runRepo, engineReader, newFakeVariableResolver(), newFakeWorkerDispatcher(true), locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("expected an unimplemented engine to be handled by failing the run, not returning an error, got: %v", err)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusFailed {
		t.Errorf("expected the run to be marked failed, got %q", got.Status)
	}
}

func TestRunDispatchService_Succeeds(t *testing.T) {
	locker := newFakeWorkspaceLocker()
	runRepo := newFakeRunRepo(locker)
	run, _ := domain.NewRun(testOrgID, testWorkspaceID, "user-1")
	runRepo.put(run)
	_, _ = locker.TryLock(context.Background(), testOrgID, testWorkspaceID, run.ID)
	engineReader := newFakeWorkspaceEngineReader()
	engineReader.set(testOrgID, testWorkspaceID, "compose")
	resolver := newFakeVariableResolver()
	resolver.set(testOrgID, testWorkspaceID, "compose_file", "version: '3'")
	dispatcher := newFakeWorkerDispatcher(true)
	svc := application.NewRunDispatchService(runRepo, engineReader, resolver, dispatcher, locker)

	err := svc.HandleEvent(context.Background(), runQueuedEvent(run.ID, testWorkspaceID))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if dispatcher.calls != 1 {
		t.Errorf("expected the dispatcher to be called once, got %d", dispatcher.calls)
	}
	got, _ := runRepo.GetByID(context.Background(), testOrgID, run.ID)
	if got.Status != domain.RunStatusApplying {
		t.Errorf("expected the run to be left in applying (TryStartApplying's claim), got %q", got.Status)
	}
}
