package application

import (
	"context"
	"encoding/json"

	"platform-of-platform/internal/execution/domain"
	"platform-of-platform/internal/platform/outbox"
)

// configVariableKeyByEngine maps each engine the Worker actually
// implements (cmd/worker, internal/worker/engine) to the Variables key
// its config content is resolved from (docs/architecture/03-domain-
// model.md §7's cascade, reused here rather than building a GitOps/
// upload flow this codebase doesn't have) - a real, named stand-in for
// the "config_bundle" a real GitOps checkout would supply, one key per
// engine since a Terraform config and a Compose file are never the same
// Variable. An ExecutionEngine enum value with no entry here (the
// remaining three: helm, kubespray, kubernetes) is handled the same way
// a genuinely-missing Variable is below - there's no Worker-side engine
// for it yet regardless of what a Variable might contain.
var configVariableKeyByEngine = map[string]string{
	"compose":   "compose_file",
	"terraform": "terraform_config",
	"opentofu":  "opentofu_config",
	"ansible":   "ansible_playbook",
	"packer":    "packer_template",
}

// RunDispatchService.HandleEvent implements outbox.Handler - subscribes
// to RunQueued events the exact same way Audit's RecordEntryService
// does (see main.go's composed handler), reusing the Outbox as the
// fan-out mechanism instead of a second, cross-org polling loop over
// the runs table directly - that would need an unscoped (root-
// privileged) query this codebase's runtime deliberately never uses
// (internal/platform/config's DatabaseURL/AppDatabaseURL split).
// This is the Run Dispatcher docs/architecture/17-workers.md §7
// describes: match a queued Run's Workspace engine against a connected
// Worker's supported_engines.
type RunDispatchService struct {
	runRepo          RunRepository
	engineReader     WorkspaceEngineReader
	variableResolver VariableResolver
	dispatcher       WorkerDispatcher
	locker           WorkspaceLocker
}

func NewRunDispatchService(runRepo RunRepository, engineReader WorkspaceEngineReader, variableResolver VariableResolver, dispatcher WorkerDispatcher, locker WorkspaceLocker) *RunDispatchService {
	return &RunDispatchService{runRepo: runRepo, engineReader: engineReader, variableResolver: variableResolver, dispatcher: dispatcher, locker: locker}
}

func (s *RunDispatchService) HandleEvent(ctx context.Context, event outbox.Event) error {
	if event.EventType != "RunQueued" {
		return nil
	}

	var payload struct {
		TargetID    string `json:"target_id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	runID := payload.TargetID
	organizationID := event.OrganizationID
	workspaceID := payload.WorkspaceID

	// The atomic claim - see TryStartApplying's own doc comment for why
	// this has to be a compare-and-swap, not a read-then-write: a
	// redelivered RunQueued event (the Outbox Relay's at-least-once
	// guarantee) must not double-dispatch the same Run.
	started, err := s.runRepo.TryStartApplying(ctx, organizationID, runID, workspaceID)
	if err != nil {
		return err
	}
	if !started {
		// Already dispatched by an earlier delivery of this same event,
		// or the Run was canceled before dispatch ever ran - either way,
		// a safe, expected no-op, not an error.
		return nil
	}

	run, err := s.runRepo.GetByID(ctx, organizationID, runID)
	if err != nil {
		return err
	}

	engine, err := s.engineReader.GetExecutionEngine(ctx, organizationID, workspaceID)
	if err != nil {
		return err
	}

	configVariableKey, hasEngine := configVariableKeyByEngine[engine]
	if !hasEngine {
		// A real ExecutionEngine enum value (Workspace creation already
		// validated it), but no Worker-side engine implements it yet -
		// same non-transient "fail now, don't retry forever" posture as
		// a genuinely-missing config Variable below, since no
		// configVariableKeyByEngine entry will ever appear without a
		// code change.
		return s.fail(ctx, run, workspaceID)
	}

	configBundle, found, err := s.variableResolver.ResolveValue(ctx, organizationID, workspaceID, configVariableKey, run.TriggeredBy)
	if err != nil {
		return err
	}
	if !found {
		// Missing configuration isn't transient - retrying won't fix it,
		// so this fails the Run outright rather than reverting to
		// queued for a redelivery to retry forever.
		return s.fail(ctx, run, workspaceID)
	}

	dispatched, err := s.dispatcher.Dispatch(ctx, runID, organizationID, workspaceID, engine, configBundle)
	if err != nil {
		return err
	}
	if dispatched {
		return nil
	}

	// No connected Worker supports this engine right now - this *is*
	// transient (a Worker might connect soon), so revert the claim and
	// return a real error, leaving the RunQueued event unpublished so
	// the Relay's own at-least-once redelivery retries it later. A
	// dedicated Stale Run Reaper (docs/architecture/07-module-
	// execution.md §3's own name for "the single most commonly-missed
	// piece in a first-pass execution-engine design") would be the
	// stronger version of this; retry-via-redelivery is this codebase's
	// honestly-reduced substitute, not a silent gap.
	if revertErr := s.runRepo.RevertToQueued(ctx, organizationID, runID); revertErr != nil {
		return revertErr
	}
	return domain.ErrNoWorkerAvailable
}

func (s *RunDispatchService) fail(ctx context.Context, run *domain.Run, workspaceID string) error {
	if err := run.MarkFailed(); err != nil {
		return err
	}
	if err := s.runRepo.Update(ctx, run, "system"); err != nil {
		return err
	}
	return s.locker.Unlock(ctx, run.OrganizationID, workspaceID, run.ID)
}
