// Package grpc is the Execution context's gRPC adapter - the Worker
// side of docs/architecture/04-api-design.md §10 /
// docs/architecture/17-workers.md.
package grpc

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

// workerInstanceTTL/workerInstanceHeartbeat - how long a
// worker-instance:<id> Redis key survives without a refresh, and how
// often Heartbeat refreshes it. A real, bounded staleness window (not
// indefinite) for the one case a graceful deregister() can't cover: an
// instance that dies without ever running its own deferred cleanup
// (SIGKILL, OOM-killed) - the same "bounded, not unbounded" posture
// already applied to runToWorker/idempotency_keys.
const (
	workerInstanceTTL       = 30 * time.Second
	workerInstanceHeartbeat = 10 * time.Second
)

type workerEntry struct {
	supportedEngines map[string]bool
	jobs             chan *pb.WorkerCommand
}

// Registry is the directory of connected Workers - the Worker
// gRPC-stream side (workers map, jobs channels) is unavoidably
// process-local: a live gRPC stream literally exists inside one
// goroutine in one process, there's nothing to share. What used to be
// entirely process-local too - runID -> Worker routing, and "which
// instance is this Worker even connected to" - is now mirrored into
// Redis (docs/architecture/05-database.md §5's own "cache/coordination,
// never system-of-record" role for it), closing the real multi-instance
// HA gap this type's own doc comment used to name: previously, a Cancel
// request landing on an instance that didn't itself dispatch the Run
// (a different replica did) had no way to find it at all. register()'s
// activeRunIDs handling still recovers *same-instance* restart state
// directly from the Worker's own report, unrelated to the Redis-backed
// cross-instance path below.
type Registry struct {
	mu          sync.RWMutex
	workers     map[string]*workerEntry
	runToWorker map[string]string

	// instanceID identifies this Control Plane replica - main.go derives
	// it from os.Hostname(), which Docker sets to a real, per-container
	// random ID by default, a genuine unique-per-replica identity with
	// no extra coordination needed to obtain one.
	instanceID string
	redis      *redis.Client
	logger     *slog.Logger
}

func NewRegistry(instanceID string, redisClient *redis.Client, logger *slog.Logger) *Registry {
	return &Registry{
		workers:     make(map[string]*workerEntry),
		runToWorker: make(map[string]string),
		instanceID:  instanceID,
		redis:       redisClient,
		logger:      logger,
	}
}

func runWorkerKey(runID string) string         { return "worker-registry:run-worker:" + runID }
func workerInstanceKey(workerID string) string { return "worker-registry:worker-instance:" + workerID }
func cancelChannel(instanceID string) string   { return "worker-registry:cancel:" + instanceID }

// register also takes activeRunIDs - the Run IDs the connecting Worker
// says it's still actually running (RegisterRequest.active_run_ids).
// Rebuilding runToWorker from this on every Register, not just on
// Dispatch, is what makes Cancel routing survive a Control Plane
// restart: the Worker itself is the only durable source of truth for
// "what am I currently running." The same rebuild now also republishes
// to Redis, so a Worker that reconnects to a *different* replica after
// its original instance restarted correctly updates every other
// replica's view of who owns it.
func (r *Registry) register(ctx context.Context, workerID string, supportedEngines []string, activeRunIDs []string) chan *pb.WorkerCommand {
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

	if err := r.redis.Set(ctx, workerInstanceKey(workerID), r.instanceID, workerInstanceTTL).Err(); err != nil {
		r.logger.Error("failed to publish worker ownership to redis", "worker_id", workerID, "error", err)
	}
	for _, runID := range activeRunIDs {
		if err := r.redis.Set(ctx, runWorkerKey(runID), workerID, 0).Err(); err != nil {
			r.logger.Error("failed to publish run routing to redis", "run_id", runID, "error", err)
		}
	}

	return jobs
}

func (r *Registry) deregister(workerID string) {
	r.mu.Lock()
	delete(r.workers, workerID)
	r.mu.Unlock()

	// stream.Context() (the caller, Server.StreamJobs's own defer) is
	// already Done() by the time this runs - a fresh, short-lived
	// context for this one cleanup call, same posture as main.go's own
	// shutdown-time DB/tracing cleanup calls.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.redis.Del(ctx, workerInstanceKey(workerID)).Err(); err != nil {
		r.logger.Error("failed to remove worker ownership from redis", "worker_id", workerID, "error", err)
	}
}

// Dispatch implements the Execution application layer's own
// WorkerDispatcher port (internal/execution/application/ports.go) -
// plain string parameters, not a shared struct, matching every other
// port in this codebase (IsMember, HasPermission, TryLock, ...) rather
// than introducing a data type either side would need to import from
// the other. Picks any *locally* connected Worker advertising the
// requested engine and pushes the job to its channel. Deliberately does
// NOT attempt to forward to a Worker connected to a different replica
// when there's no local match: RunDispatchService already reverts an
// undispatched Run to `queued` and returns a real error on (false, nil)
// here, which the Outbox Relay's own at-least-once redelivery retries -
// since every replica's Relay polls the *same* outbox_events table
// independently, a later retry naturally has a chance of being
// processed by whichever replica actually holds the matching Worker,
// with TryStartApplying's atomic compare-and-swap preventing a
// double-dispatch if two replicas race on the same retry. A real,
// working emergent property of the existing design, not something this
// method needs to reimplement.
func (r *Registry) Dispatch(ctx context.Context, runID, organizationID, workspaceID, executionEngine, configBundle string) (bool, error) {
	r.mu.Lock()
	var matchedWorkerID string
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
			matchedWorkerID = workerID
		default:
			// This worker's queue is full - try the next one rather
			// than blocking the whole dispatch loop on one busy Worker.
			continue
		}
		break
	}
	r.mu.Unlock()

	if matchedWorkerID == "" {
		return false, nil
	}

	if err := r.redis.Set(ctx, runWorkerKey(runID), matchedWorkerID, 0).Err(); err != nil {
		r.logger.Error("failed to publish run routing to redis", "run_id", runID, "error", err)
	}
	return true, nil
}

// CancelJob implements Execution's own WorkerCanceler port
// (internal/execution/application/ports.go) - routes a CancelJob
// command to whichever Worker Dispatch recorded as running runID.
// Tries this instance's own local routing first (the common case: the
// same replica that dispatched the Run also received the Cancel
// request); if this instance never knew about runID at all, or its
// local Worker entry is gone, falls back to the shared Redis routing
// table to find whichever *other* live replica the Worker is actually
// connected to and forwards the Cancel there via Pub/Sub - the real fix
// for the multi-instance HA gap this Registry used to name (a Cancel
// landing on a replica that didn't dispatch the Run had no way to
// deliver it before this). Both the local and forwarded paths are
// deliberately best-effort - CancelRunService's own caller already
// treats this as best-effort after the authoritative DB transition, a
// lost Pub/Sub message here is no worse than a lost local channel push
// already was.
func (r *Registry) CancelJob(ctx context.Context, runID string) (bool, error) {
	if delivered, ok := r.cancelLocal(runID); ok {
		return delivered, nil
	}

	workerID, err := r.redis.Get(ctx, runWorkerKey(runID)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	ownerInstanceID, err := r.redis.Get(ctx, workerInstanceKey(workerID)).Result()
	if err == redis.Nil {
		// The owning replica's worker-instance key has expired past
		// workerInstanceTTL without a heartbeat refresh - that replica's
		// connection to this Worker is gone (crashed, never gracefully
		// deregistered). Genuinely nothing live left to cancel anywhere.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if ownerInstanceID == r.instanceID {
		// Reached here without a local match above - a real, narrow
		// race (the Worker disconnected from this same instance between
		// the local check and this Redis read). Nothing more to do.
		return false, nil
	}

	if err := r.redis.Publish(ctx, cancelChannel(ownerInstanceID), runID).Err(); err != nil {
		return false, err
	}
	return true, nil
}

// cancelLocal attempts delivery through this instance's own in-process
// state. ok=false means this instance has no usable local routing entry
// at all (never tracked here, or its Worker disconnected) - the signal
// CancelJob uses to fall back to Redis; ok=true means this instance did
// own the routing (whether or not the channel push itself succeeded)
// and no further fallback should be attempted.
func (r *Registry) cancelLocal(runID string) (delivered bool, ok bool) {
	r.mu.Lock()
	workerID, tracked := r.runToWorker[runID]
	var jobs chan *pb.WorkerCommand
	if tracked {
		if entry, exists := r.workers[workerID]; exists {
			jobs = entry.jobs
		}
	}
	r.mu.Unlock()

	if jobs == nil {
		return false, false
	}

	cmd := &pb.WorkerCommand{Command: &pb.WorkerCommand_CancelJob{
		CancelJob: &pb.CancelJob{RunId: runID},
	}}

	delivered = false
	select {
	case jobs <- cmd:
		delivered = true
	default:
	}

	r.mu.Lock()
	delete(r.runToWorker, runID)
	r.mu.Unlock()

	// This instance genuinely owned the routing entry (whether or not
	// the channel push itself succeeded) - clean up its Redis mirror too,
	// same reasoning as Forget: leaving it behind would reopen the exact
	// unbounded-growth gap Forget was built to close, just for the new
	// Redis-mirrored state instead of the local map. Deliberately not
	// done in CancelJob's own Redis-fallback branch above - a run this
	// instance *doesn't* own is cleaned up here instead, on whichever
	// instance's cancelLocal call (direct or via SubscribeCancelForwarding)
	// actually owns it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.redis.Del(ctx, runWorkerKey(runID)).Err(); err != nil {
		r.logger.Error("failed to remove run routing from redis", "run_id", runID, "error", err)
	}

	return delivered, true
}

// SubscribeCancelForwarding implements the Runnable interface
// (docs/architecture/18-backend-structure.md §4) - the receiving half
// of CancelJob's cross-instance forward: subscribes to this replica's
// own Pub/Sub channel and, on every message, retries delivery through
// cancelLocal (this replica is, by construction, the one CancelJob's
// Redis lookup determined actually owns the Worker).
func (r *Registry) SubscribeCancelForwarding(ctx context.Context) error {
	sub := r.redis.Subscribe(ctx, cancelChannel(r.instanceID))
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if _, tracked := r.cancelLocal(msg.Payload); !tracked {
				r.logger.Error("received a forwarded cancel for a run this instance no longer tracks", "run_id", msg.Payload)
			}
		}
	}
}

// Heartbeat implements the Runnable interface - periodically refreshes
// every currently-connected local Worker's workerInstanceTTL in Redis,
// so a hard crash (no graceful deregister) only leaves a stale
// worker-instance:<id> entry around for at most workerInstanceTTL, not
// forever.
func (r *Registry) Heartbeat(ctx context.Context) error {
	ticker := time.NewTicker(workerInstanceHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.mu.RLock()
			workerIDs := make([]string, 0, len(r.workers))
			for id := range r.workers {
				workerIDs = append(workerIDs, id)
			}
			r.mu.RUnlock()

			for _, id := range workerIDs {
				if err := r.redis.Expire(ctx, workerInstanceKey(id), workerInstanceTTL).Err(); err != nil {
					r.logger.Error("failed to refresh worker ownership ttl", "worker_id", id, "error", err)
				}
			}
		}
	}
}

// Forget implements Execution's own RunTracker port
// (internal/execution/application/ports.go) - removes runID's routing
// entry, both locally and from the shared Redis table, once the
// application layer has independently confirmed the Run reached a
// terminal status by some path other than Cancel (which already
// deletes its own entries above). A harmless no-op if no entry exists
// anywhere (e.g. the Run never reached a Worker in the first place, or
// was already forgotten).
func (r *Registry) Forget(runID string) {
	r.mu.Lock()
	delete(r.runToWorker, runID)
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.redis.Del(ctx, runWorkerKey(runID)).Err(); err != nil {
		r.logger.Error("failed to remove run routing from redis", "run_id", runID, "error", err)
	}
}
