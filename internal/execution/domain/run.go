// Package domain holds the Execution context's pure Go types
// (docs/architecture/03-domain-model.md §6) - "the core workflow."
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRunNotFound       = errors.New("run not found")
	ErrWorkspaceNotFound = errors.New("workspace not found")
	ErrForbidden         = errors.New("forbidden")
	// ErrWorkspaceLocked maps to HTTP 409 - "a real conflict, retrying
	// without a state change (i.e. without the in-flight Run finishing)
	// won't help," the textbook case 409 exists for, unlike 400/403's
	// "the request itself was wrong."
	ErrWorkspaceLocked = errors.New("workspace is locked by another run")
	// ErrRunAlreadyTerminal - Cancel()'s own invariant.
	ErrRunAlreadyTerminal = errors.New("run is already in a terminal status")
	// ErrInvalidTransition - MarkApplied/MarkFailed's own invariant:
	// only a Run actually in `applying` can complete, the same
	// "compile-reachable, exhaustively testable" state-machine
	// discipline Cancel() already established.
	ErrInvalidTransition = errors.New("invalid run status transition")
	// ErrNoWorkerAvailable - RunDispatchService's own signal that
	// nothing is wrong with the Run itself, there's just no connected
	// Worker for its engine *right now*. Returned as a real error so the
	// Outbox Relay's at-least-once redelivery retries the RunQueued
	// event later, standing in for a dedicated Stale Run Reaper this
	// codebase doesn't have.
	ErrNoWorkerAvailable = errors.New("no worker available for this execution engine")
	// ErrOrganizationArchived - same meaning as tenancy/domain's own
	// sentinel, redeclared here per this codebase's no-cross-context-
	// import rule. TriggerRunService checks this before creating a new
	// Run in an archived Organization - CancelRunService deliberately
	// does NOT (stopping something already running should stay possible
	// regardless of archival status).
	ErrOrganizationArchived = errors.New("organization is archived")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// RunStatus is the closed set from docs/architecture/03-domain-model.md
// §6. This codebase's own code only ever produces `queued`, `applying`,
// `applied`, `failed`, `errored`, and `canceled` - the Worker
// (docs/architecture/17-workers.md) goes straight from queued to
// applying, skipping `planning`/`planned`/`policy_check`/
// `awaiting_approval` (real Plan/Policy/Approval flows this codebase
// hasn't built yet, deliberately not faked). The full enum is still
// modeled (and the CHECK constraint in migrations/0005_runs.up.sql
// enforces it at the schema level too) because Run.Status needs to be
// the real, complete type those future flows will write into, not a
// narrower one this slice would have to widen later.
type RunStatus string

const (
	RunStatusQueued           RunStatus = "queued"
	RunStatusPlanning         RunStatus = "planning"
	RunStatusPlanned          RunStatus = "planned"
	RunStatusPolicyCheck      RunStatus = "policy_check"
	RunStatusAwaitingApproval RunStatus = "awaiting_approval"
	RunStatusApplying         RunStatus = "applying"
	RunStatusApplied          RunStatus = "applied"
	RunStatusFailed           RunStatus = "failed"
	RunStatusErrored          RunStatus = "errored"
	RunStatusCanceled         RunStatus = "canceled"
)

func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunStatusApplied, RunStatusFailed, RunStatusErrored, RunStatusCanceled:
		return true
	}
	return false
}

// RunTrigger is the closed set from Stage 3 §6. Only manual/api are
// ever actually produced by this codebase yet (vcs_push/vcs_pr need
// GitOps, scheduled needs a scheduler - neither exists here), same
// "modeled fully, implemented partially" posture as RunStatus above.
type RunTrigger string

const (
	RunTriggerManual    RunTrigger = "manual"
	RunTriggerVCSPush   RunTrigger = "vcs_push"
	RunTriggerVCSPR     RunTrigger = "vcs_pr"
	RunTriggerScheduled RunTrigger = "scheduled"
	RunTriggerAPI       RunTrigger = "api"
)

// StaleRunCandidate is what the Stale Run Reaper
// (docs/architecture/07-module-execution.md §3) works with - read
// straight from outbox_events (no RLS on that table, see
// RunRepository.FindStaleApplyingRuns's own comment), a genuine
// cross-org scan, unlike every other Run query in this codebase which
// is scoped to one organizationID at a time.
type StaleRunCandidate struct {
	OrganizationID string
	RunID          string
	WorkspaceID    string
}

// Run is the Execution context's aggregate root
// (docs/architecture/03-domain-model.md §6).
type Run struct {
	ID             string
	OrganizationID string
	WorkspaceID    string
	Trigger        RunTrigger
	TriggeredBy    string // a real user id, or the literal "system" - see migrations/0005_runs.up.sql
	Status         RunStatus
	PlanOutputRef  *string
	ApplyOutputRef *string
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

// NewRun always starts a Run at `queued`, triggered manually by the
// authenticated caller (this codebase's only real trigger path) - per
// Stage 3 §6, this is also the moment that (via the caller's own
// WorkspaceLocker.TryLock call, not this constructor) claims the
// Workspace's lock.
func NewRun(organizationID, workspaceID, triggeredByUserID string) (*Run, error) {
	if organizationID == "" || workspaceID == "" {
		return nil, &ValidationError{Message: "organization_id and workspace_id are required"}
	}
	if triggeredByUserID == "" {
		return nil, &ValidationError{Message: "triggered_by is required"}
	}

	return &Run{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Trigger:        RunTriggerManual,
		TriggeredBy:    triggeredByUserID,
		Status:         RunStatusQueued,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// Cancel is the one real state transition this slice exercises end to
// end (Stage 7 §1's "state machine as methods on Run" pattern, applied
// to the single transition available without a Worker to drive the
// others) - a compile-reachable, exhaustively testable rule: canceling
// an already-terminal Run is rejected, not silently accepted.
func (r *Run) Cancel() error {
	if r.Status.IsTerminal() {
		return ErrRunAlreadyTerminal
	}

	now := time.Now().UTC()
	r.Status = RunStatusCanceled
	r.FinishedAt = &now
	return nil
}

// MarkApplied / MarkFailed are the two real terminal transitions the
// Worker reports via ReportJobStatus - both only valid from `applying`,
// enforced here the same way Cancel() enforces its own precondition,
// not left to the caller (WorkerReportService) to remember to check.
func (r *Run) MarkApplied() error {
	if r.Status != RunStatusApplying {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	r.Status = RunStatusApplied
	r.FinishedAt = &now
	return nil
}

func (r *Run) MarkFailed() error {
	if r.Status != RunStatusApplying {
		return ErrInvalidTransition
	}
	now := time.Now().UTC()
	r.Status = RunStatusFailed
	r.FinishedAt = &now
	return nil
}
