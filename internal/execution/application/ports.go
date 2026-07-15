package application

import (
	"context"
	"time"

	"platform-of-platform/internal/execution/domain"
)

type RunRepository interface {
	Create(ctx context.Context, run *domain.Run) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Run, error)
	ListByWorkspace(ctx context.Context, organizationID, workspaceID string) ([]*domain.Run, error)
	// Update persists a Run whose in-memory state was already mutated by
	// a domain method (Run.Cancel()) - same "domain decides, repository
	// just persists" split as every other write in this codebase.
	// actorUserID is attached to the outbox event the adapter writes in
	// the same transaction, same reasoning as OrganizationRepository.Create.
	Update(ctx context.Context, run *domain.Run, actorUserID string) error
	// TryStartApplying is a real atomic compare-and-swap (UPDATE ... WHERE
	// status = 'queued'), not a read-Run/call-a-domain-method/write
	// round trip - RunDispatchService needs this specific shape because
	// it's driven by the Outbox Relay's at-least-once redelivery
	// (internal/platform/outbox/relay.go): a redelivered RunQueued event
	// must be a safe no-op, not a second dispatch of the same Run. Same
	// "atomic conditional update, not read-then-write" reasoning as
	// WorkspaceLocker.TryLock. Returns (false, nil) if the Run wasn't
	// in `queued` when this ran (already dispatched, or canceled first).
	// workspaceID is attached to the RunApplying outbox event so the
	// Stale Run Reaper (reap_stale_runs.go) can unlock the right
	// Workspace without a second lookup.
	TryStartApplying(ctx context.Context, organizationID, runID, workspaceID string) (bool, error)
	// RevertToQueued undoes TryStartApplying when dispatch itself then
	// fails to find a connected Worker - a best-effort compensation, not
	// a full saga.
	RevertToQueued(ctx context.Context, organizationID, runID string) error
	// FindStaleApplyingRuns and MarkErroredIfStillApplying are the Stale
	// Run Reaper's own two operations (reap_stale_runs.go) - see that
	// file's own doc comment for why a Run whose Worker died *after*
	// successfully receiving its JobAssignment needs a dedicated
	// mechanism, not just TriggerRunService's "no Worker was connected
	// at dispatch time" retry path.
	FindStaleApplyingRuns(ctx context.Context, olderThan time.Time) ([]domain.StaleRunCandidate, error)
	MarkErroredIfStillApplying(ctx context.Context, organizationID, runID string) (bool, error)
}

// WorkspaceLocker is this context's port into Workspace for the one
// property Stage 3 §6 calls out as load-bearing: "only one Run may be
// in a non-terminal status per Workspace at a time - this IS the
// Workspace's lock_status." TryLock returns (false, nil), not an error,
// when the workspace was already locked - same (bool, error) shape as
// every other cross-context check in this codebase.
type WorkspaceLocker interface {
	TryLock(ctx context.Context, organizationID, workspaceID, runID string) (bool, error)
	Unlock(ctx context.Context, organizationID, workspaceID, runID string) error
}

// WorkspaceChecker, MembershipChecker, PermissionChecker - same shape
// and reasoning as every other context's own-declared ports into
// Tenancy/RBAC/Workspace (docs/architecture/18-backend-structure.md §3).
type WorkspaceChecker interface {
	WorkspaceExists(ctx context.Context, organizationID, projectID, workspaceID string) (bool, error)
}

type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// ScopedPermissionChecker is the RBAC-at-project/workspace-scope port
// (docs/architecture/03-domain-model.md §4's "a binding at a higher
// scope implies the grant at every resource beneath it") -
// TriggerRunService/CancelRunService use this instead of the plain
// PermissionChecker above so a RoleBinding at workspace or project
// scope (not just organization) actually grants workspace:apply. nil
// projectID/workspaceID mean "don't check that level" - both non-nil
// here since a Run always has both.
type ScopedPermissionChecker interface {
	HasPermissionAtScope(ctx context.Context, organizationID, userID, permission string, projectID, workspaceID *string) (bool, error)
}

// OrganizationChecker - this context's port into Tenancy for "is this
// Organization archived" (docs/architecture/13-module-identity-rbac-
// tenancy.md §1). TriggerRunService checks this before creating a new
// Run - same enforcement point Project/Workspace/Variable creation
// already apply.
type OrganizationChecker interface {
	IsArchived(ctx context.Context, organizationID string) (bool, error)
}

// WorkspaceEngineReader is RunDispatchService's own port into Workspace -
// it needs the Workspace's ExecutionEngine to pick a matching connected
// Worker, and nothing else about the Workspace, so it asks for exactly
// that (not the full domain.Workspace - Execution never imports
// workspace/domain).
type WorkspaceEngineReader interface {
	GetExecutionEngine(ctx context.Context, organizationID, workspaceID string) (string, error)
}

// VariableResolver is RunDispatchService's port into Variables - the
// config_bundle a JobAssignment carries is sourced from the Variables
// context's own cascade resolution (a real, already-proven mechanism)
// rather than a GitOps/upload flow this codebase hasn't built yet.
// Returns (value, false, nil), not an error, when no variable is found -
// same (bool, error) shape as every cross-context check in this
// codebase.
type VariableResolver interface {
	ResolveValue(ctx context.Context, organizationID, workspaceID, key, requestingUserID string) (string, bool, error)
}

// WorkerDispatcher is RunDispatchService's port into the gRPC adapter's
// Registry (internal/execution/adapters/grpc) - plain string parameters
// again, not a shared struct, so neither side needs to import a type
// from the other (docs/architecture/18-backend-structure.md §3).
// Returns (false, nil) when no connected Worker supports the requested
// engine right now.
type WorkerDispatcher interface {
	Dispatch(ctx context.Context, runID, organizationID, workspaceID, executionEngine, configBundle string) (bool, error)
}

// WorkerCanceler is CancelRunService's port into the gRPC adapter's
// Registry (docs/architecture/17-workers.md §6) - a real Cancel command
// reaching the Worker that's actually running the Job, not just a DB
// status flip. Returns (false, nil), not an error, when no Worker is
// currently tracked as running this Run (already finished, never
// dispatched, or its Worker disconnected) - CancelRunService's own DB
// transition already happened before this is ever called, so this
// failing to find a live Worker to notify isn't itself a failure of the
// cancel operation.
type WorkerCanceler interface {
	CancelJob(ctx context.Context, runID string) (bool, error)
}

// RunTracker is WorkerReportService's and StaleRunReaperService's own
// port into the gRPC adapter's Registry - closes the "runToWorker grows
// unboundedly" gap the Registry's own doc comment used to name:
// CancelJob already forgets its own routing entry, but a Run that
// completes normally (a real Worker report) or gets reaped (a Worker
// that died mid-Job and never reported) previously left its entry
// behind forever. Forget is a plain, error-free cleanup call - there's
// nothing a caller could usefully do differently if the entry was
// already gone (Run never dispatched, or already forgotten by another
// path), so this deliberately isn't shaped like every other (bool,
// error) cross-context check in this codebase.
type RunTracker interface {
	Forget(runID string)
}
