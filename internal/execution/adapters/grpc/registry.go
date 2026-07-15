// Package grpc is the Execution context's gRPC adapter - the Worker
// side of docs/architecture/04-api-design.md §10 /
// docs/architecture/17-workers.md.
package grpc

import (
	"context"
	"sync"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

type workerEntry struct {
	supportedEngines map[string]bool
	jobs             chan *pb.WorkerCommand
}

// Registry is the in-memory, single-process directory of connected
// Workers - a real, legitimate implementation for a single Control
// Plane instance (no HA multi-instance concern in this walking
// skeleton yet - a future multi-instance Control Plane would need this
// state shared, not local; Redis is exactly the "cache/coordination,
// never system-of-record" role docs/architecture/05-database.md §5
// already reserves for it, the natural place this would move to).
// register()'s active_run_ids handling (below) recovers same-instance
// state across a Control Plane *restart* - Workers re-report what
// they're running - but that's orthogonal to true multi-instance
// sharing and doesn't attempt to solve it.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*workerEntry
	// runToWorker routes a later CancelJob back to whichever Worker is
	// currently running it - populated by Dispatch, consulted (and
	// opportunistically cleared) by CancelJob. Previously a known, flagged
	// gap: an entry for a Run that completes *without* ever being
	// canceled was never removed. Now closed via Forget below, called
	// from both real "this Run just reached a terminal status" hooks the
	// application layer has: WorkerReportService (applied/failed, a real
	// Worker report) and StaleRunReaperService (errored, a Worker that
	// died mid-Job and never reported at all) - between CancelJob's own
	// delete and these two, every path a Run can take out of `applying`
	// now forgets its routing entry.
	runToWorker map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		workers:     make(map[string]*workerEntry),
		runToWorker: make(map[string]string),
	}
}

// register also takes activeRunIDs - the Run IDs the connecting Worker
// says it's still actually running (RegisterRequest.active_run_ids).
// Rebuilding runToWorker from this on every Register, not just on
// Dispatch, is what makes Cancel routing survive a Control Plane
// restart: the Worker itself is the only durable source of truth for
// "what am I currently running," since this Registry's own state is
// never persisted (docs/architecture/17-workers.md's known no-HA gap -
// this doesn't fix multi-instance sharing, only same-instance restart).
func (r *Registry) register(workerID string, supportedEngines []string, activeRunIDs []string) chan *pb.WorkerCommand {
	engines := make(map[string]bool, len(supportedEngines))
	for _, e := range supportedEngines {
		engines[e] = true
	}
	jobs := make(chan *pb.WorkerCommand, 16)

	r.mu.Lock()
	r.workers[workerID] = &workerEntry{supportedEngines: engines, jobs: jobs}
	for _, runID := range activeRunIDs {
		r.runToWorker[runID] = workerID
	}
	r.mu.Unlock()

	return jobs
}

func (r *Registry) deregister(workerID string) {
	r.mu.Lock()
	delete(r.workers, workerID)
	r.mu.Unlock()
}

// Dispatch implements the Execution application layer's own
// WorkerDispatcher port (internal/execution/application/ports.go) -
// plain string parameters, not a shared struct, matching every other
// port in this codebase (IsMember, HasPermission, TryLock, ...) rather
// than introducing a data type either side would need to import from
// the other. Picks any connected Worker advertising the requested
// engine and pushes the job to its channel, recording the routing entry
// CancelJob later needs. Returns (false, nil), not an error, when no
// matching Worker is currently connected - "the answer is no," not a
// failure, same (bool, error) shape as every cross-context check in this
// codebase; RunDispatchService decides what that means (retry later,
// via the outbox's own at-least-once redelivery).
func (r *Registry) Dispatch(ctx context.Context, runID, organizationID, workspaceID, executionEngine, configBundle string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for workerID, w := range r.workers {
		if !w.supportedEngines[executionEngine] {
			continue
		}
		cmd := &pb.WorkerCommand{Command: &pb.WorkerCommand_JobAssignment{
			JobAssignment: &pb.JobAssignment{
				RunId:           runID,
				OrganizationId:  organizationID,
				WorkspaceId:     workspaceID,
				ExecutionEngine: executionEngine,
				ConfigBundle:    configBundle,
			},
		}}
		select {
		case w.jobs <- cmd:
			r.runToWorker[runID] = workerID
			return true, nil
		default:
			// This worker's queue is full - try the next one rather
			// than blocking the whole dispatch loop on one busy Worker.
			continue
		}
	}

	return false, nil
}

// CancelJob implements Execution's own WorkerCanceler port
// (internal/execution/application/ports.go) - routes a CancelJob
// command to whichever Worker Dispatch recorded as running runID.
// Returns (false, nil), not an error, when no Worker is currently
// tracked for this Run - it may have already finished, never been
// dispatched yet, or its Worker disconnected in the meantime; none of
// those are failures, there's just nothing live left to cancel.
func (r *Registry) CancelJob(ctx context.Context, runID string) (bool, error) {
	r.mu.Lock()
	workerID, ok := r.runToWorker[runID]
	if ok {
		delete(r.runToWorker, runID)
	}
	var jobs chan *pb.WorkerCommand
	if ok {
		if entry, exists := r.workers[workerID]; exists {
			jobs = entry.jobs
		}
	}
	r.mu.Unlock()

	if jobs == nil {
		return false, nil
	}

	cmd := &pb.WorkerCommand{Command: &pb.WorkerCommand_CancelJob{
		CancelJob: &pb.CancelJob{RunId: runID},
	}}

	select {
	case jobs <- cmd:
		return true, nil
	default:
		return false, nil
	}
}

// Forget implements Execution's own RunTracker port
// (internal/execution/application/ports.go) - removes runID's routing
// entry once the application layer has independently confirmed the Run
// reached a terminal status by some path other than Cancel (which
// already deletes its own entry above). A harmless no-op if no entry
// exists (e.g. the Run never reached a Worker in the first place, or
// was already forgotten).
func (r *Registry) Forget(runID string) {
	r.mu.Lock()
	delete(r.runToWorker, runID)
	r.mu.Unlock()
}
