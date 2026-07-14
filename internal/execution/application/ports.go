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
