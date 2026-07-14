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
	jobs             chan *pb.JobAssignment
}

// Registry is the in-memory, single-process directory of connected
// Workers - a real, legitimate implementation for a single Control
// Plane instance (no HA multi-instance concern in this walking
// skeleton yet). A future multi-instance Control Plane would need this
// state shared, not local - Redis is exactly the "cache/coordination,
// never system-of-record" role docs/architecture/05-database.md §5
// already reserves for it, the natural place this would move to.
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*workerEntry
}

func NewRegistry() *Registry {
	return &Registry{workers: make(map[string]*workerEntry)}
}

func (r *Registry) register(workerID string, supportedEngines []string) chan *pb.JobAssignment {
	engines := make(map[string]bool, len(supportedEngines))
	for _, e := range supportedEngines {
		engines[e] = true
	}
	jobs := make(chan *pb.JobAssignment, 16)

	r.mu.Lock()
	r.workers[workerID] = &workerEntry{supportedEngines: engines, jobs: jobs}
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
// engine and pushes the job to its channel. Returns (false, nil), not
// an error, when no matching Worker is currently connected - "the
// answer is no," not a failure, same (bool, error) shape as every
// cross-context check in this codebase; RunDispatchService decides what
// that means (retry later, via the outbox's own at-least-once
// redelivery).
func (r *Registry) Dispatch(ctx context.Context, runID, organizationID, workspaceID, executionEngine, configBundle string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, w := range r.workers {
		if !w.supportedEngines[executionEngine] {
			continue
		}
		select {
		case w.jobs <- &pb.JobAssignment{
			RunId:           runID,
			OrganizationId:  organizationID,
			WorkspaceId:     workspaceID,
			ExecutionEngine: executionEngine,
			ConfigBundle:    configBundle,
		}:
			return true, nil
		default:
			// This worker's queue is full - try the next one rather
			// than blocking the whole dispatch loop on one busy Worker.
			continue
		}
	}

	return false, nil
}
