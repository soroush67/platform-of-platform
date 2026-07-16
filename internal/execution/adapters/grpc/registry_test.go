package grpc

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
	"platform-of-platform/internal/platform/dbtest"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recvCommand reads one command off jobs with a bounded wait - every
// real Worker connection is driven by Server.StreamJobs's own select on
// this exact channel, so reading from it directly is the real interface
// this Registry pushes work through, not an implementation detail.
func recvCommand(t *testing.T, jobs chan *pb.WorkerCommand) *pb.WorkerCommand {
	t.Helper()
	select {
	case cmd := <-jobs:
		return cmd
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a command on the jobs channel")
		return nil
	}
}

func TestRegistry_Dispatch_LocalMatch(t *testing.T) {
	ctx := context.Background()
	redisClient := dbtest.RedisClient(t)
	reg := NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())

	jobs := reg.register(ctx, "worker-1", []string{"compose"}, nil)
	t.Cleanup(func() { reg.deregister("worker-1") })

	runID := uuid.NewString()
	dispatched, err := reg.Dispatch(ctx, runID, "org-1", "ws-1", "compose", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !dispatched {
		t.Fatal("expected Dispatch to find the locally registered matching worker")
	}
	t.Cleanup(func() { reg.Forget(runID) })

	cmd := recvCommand(t, jobs)
	assignment := cmd.GetJobAssignment()
	if assignment == nil || assignment.RunId != runID {
		t.Fatalf("expected a JobAssignment for run %s, got %+v", runID, cmd)
	}
	if assignment.ConfigBundle != "bundle" || assignment.CredentialBundle != "cred" {
		t.Fatalf("expected ConfigBundle/CredentialBundle to flow through unchanged, got config=%q credential=%q", assignment.ConfigBundle, assignment.CredentialBundle)
	}

	got, err := redisClient.Get(ctx, runWorkerKey(runID)).Result()
	if err != nil {
		t.Fatalf("redis GET run-worker: %v", err)
	}
	if got != "worker-1" {
		t.Errorf("expected the Redis run-worker mirror to point at worker-1, got %q", got)
	}
}

func TestRegistry_Dispatch_NoMatchingEngineReturnsFalse(t *testing.T) {
	ctx := context.Background()
	redisClient := dbtest.RedisClient(t)
	reg := NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())

	reg.register(ctx, "worker-1", []string{"compose"}, nil)
	t.Cleanup(func() { reg.deregister("worker-1") })

	dispatched, err := reg.Dispatch(ctx, uuid.NewString(), "org-1", "ws-1", "terraform", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatched {
		t.Error("expected Dispatch to fail when no connected worker supports the requested engine")
	}
}

// TestRegistry_Dispatch_SkipsAFullQueueAndTriesTheNextWorker is the real
// regression test for Dispatch's own doc comment: "This worker's queue
// is full - try the next one rather than blocking the whole dispatch
// loop." Fills worker-1's real 16-slot buffered channel by hand, then
// verifies a Dispatch call still succeeds via worker-2.
func TestRegistry_Dispatch_SkipsAFullQueueAndTriesTheNextWorker(t *testing.T) {
	ctx := context.Background()
	redisClient := dbtest.RedisClient(t)
	reg := NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())

	fullJobs := reg.register(ctx, "worker-full", []string{"compose"}, nil)
	emptyJobs := reg.register(ctx, "worker-empty", []string{"compose"}, nil)
	t.Cleanup(func() {
		reg.deregister("worker-full")
		reg.deregister("worker-empty")
	})
	for i := 0; i < cap(fullJobs); i++ {
		fullJobs <- &pb.WorkerCommand{}
	}

	runID := uuid.NewString()
	dispatched, err := reg.Dispatch(ctx, runID, "org-1", "ws-1", "compose", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !dispatched {
		t.Fatal("expected Dispatch to succeed by routing around the full worker")
	}
	t.Cleanup(func() { reg.Forget(runID) })

	select {
	case cmd := <-emptyJobs:
		if cmd.GetJobAssignment() == nil || cmd.GetJobAssignment().RunId != runID {
			t.Errorf("expected the JobAssignment to land on worker-empty, got %+v", cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected worker-empty to receive the JobAssignment that worker-full's full queue rejected")
	}
}

func TestRegistry_CancelJob_LocalDelivery(t *testing.T) {
	ctx := context.Background()
	redisClient := dbtest.RedisClient(t)
	reg := NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())

	jobs := reg.register(ctx, "worker-1", []string{"compose"}, nil)
	t.Cleanup(func() { reg.deregister("worker-1") })

	runID := uuid.NewString()
	if _, err := reg.Dispatch(ctx, runID, "org-1", "ws-1", "compose", "bundle", "cred"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	recvCommand(t, jobs) // drain the JobAssignment

	delivered, err := reg.CancelJob(ctx, runID)
	if err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if !delivered {
		t.Fatal("expected CancelJob to deliver locally to the worker that received the JobAssignment")
	}

	cmd := recvCommand(t, jobs)
	if cmd.GetCancelJob() == nil || cmd.GetCancelJob().RunId != runID {
		t.Fatalf("expected a CancelJob command for run %s, got %+v", runID, cmd)
	}

	if _, err := redisClient.Get(ctx, runWorkerKey(runID)).Result(); err == nil {
		t.Error("expected the Redis run-worker mirror to be deleted after a successful local cancel")
	}
}

func TestRegistry_CancelJob_UnknownRunReturnsFalse(t *testing.T) {
	redisClient := dbtest.RedisClient(t)
	reg := NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())

	delivered, err := reg.CancelJob(context.Background(), uuid.NewString())
	if err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if delivered {
		t.Error("expected CancelJob to return false for a run nobody has ever tracked")
	}
}

func TestRegistry_Forget_RemovesLocalAndRedisState(t *testing.T) {
	ctx := context.Background()
	redisClient := dbtest.RedisClient(t)
	reg := NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())

	reg.register(ctx, "worker-1", []string{"compose"}, nil)
	t.Cleanup(func() { reg.deregister("worker-1") })

	runID := uuid.NewString()
	if _, err := reg.Dispatch(ctx, runID, "org-1", "ws-1", "compose", "bundle", "cred"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	reg.Forget(runID)

	if _, err := redisClient.Get(ctx, runWorkerKey(runID)).Result(); err == nil {
		t.Error("expected the Redis run-worker mirror to be gone after Forget")
	}

	// The local map entry is gone too - CancelJob now has nothing local
	// to find, and Redis has nothing either, so it must report false.
	delivered, err := reg.CancelJob(ctx, runID)
	if err != nil {
		t.Fatalf("CancelJob after Forget: %v", err)
	}
	if delivered {
		t.Error("expected CancelJob to find nothing left to cancel after Forget")
	}
}

func TestRegistry_RegisterAndDeregister_PublishesAndClearsWorkerOwnership(t *testing.T) {
	ctx := context.Background()
	redisClient := dbtest.RedisClient(t)
	instanceID := "instance-" + uuid.NewString()
	reg := NewRegistry(instanceID, redisClient, discardLogger())

	reg.register(ctx, "worker-1", []string{"compose"}, nil)

	owner, err := redisClient.Get(ctx, workerInstanceKey("worker-1")).Result()
	if err != nil {
		t.Fatalf("redis GET worker-instance: %v", err)
	}
	if owner != instanceID {
		t.Errorf("expected worker-1's Redis ownership to point at %q, got %q", instanceID, owner)
	}
	ttl, err := redisClient.TTL(ctx, workerInstanceKey("worker-1")).Result()
	if err != nil {
		t.Fatalf("redis TTL: %v", err)
	}
	if ttl <= 0 || ttl > workerInstanceTTL {
		t.Errorf("expected a real, bounded TTL (0 < ttl <= %s), got %s", workerInstanceTTL, ttl)
	}

	reg.deregister("worker-1")

	if _, err := redisClient.Get(ctx, workerInstanceKey("worker-1")).Result(); err == nil {
		t.Error("expected worker-1's Redis ownership key to be gone after deregister")
	}
}

// TestRegistry_CancelJob_ForwardsCrossInstanceViaRedis is the real,
// automated version of this session's own manual two-container curl
// verification: two independent Registry instances (simulating two
// Control Plane replicas) sharing one real Redis. Instance B receives a
// Cancel for a Run only Instance A actually dispatched (and holds the
// live Worker connection for) - proving the Redis-backed lookup +
// Pub/Sub forward + SubscribeCancelForwarding's own local retry all
// genuinely work together, not just each piece in isolation.
func TestRegistry_CancelJob_ForwardsCrossInstanceViaRedis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	redisClient := dbtest.RedisClient(t)

	instanceA := "instance-a-" + uuid.NewString()
	instanceB := "instance-b-" + uuid.NewString()
	regA := NewRegistry(instanceA, redisClient, discardLogger())
	regB := NewRegistry(instanceB, redisClient, discardLogger())

	jobsA := regA.register(ctx, "worker-1", []string{"compose"}, nil)
	t.Cleanup(func() { regA.deregister("worker-1") })

	runID := uuid.NewString()
	dispatched, err := regA.Dispatch(ctx, runID, "org-1", "ws-1", "compose", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch (instance A): %v", err)
	}
	if !dispatched {
		t.Fatal("expected instance A to dispatch to its own locally connected worker")
	}
	recvCommand(t, jobsA) // drain the JobAssignment

	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	subReady := make(chan struct{})
	go func() {
		close(subReady)
		_ = regA.SubscribeCancelForwarding(subCtx)
	}()
	<-subReady
	// A real, small settle window for the SUBSCRIBE to actually reach
	// Redis before instance B publishes - Redis Pub/Sub delivers only to
	// subscribers already registered at publish time, no queuing for a
	// subscriber that hasn't connected yet.
	time.Sleep(200 * time.Millisecond)

	// Instance B has zero local knowledge of this run - its own
	// runToWorker map was never touched - so this exercises the full
	// Redis-lookup-then-forward path, not the local fast path.
	delivered, err := regB.CancelJob(ctx, runID)
	if err != nil {
		t.Fatalf("CancelJob (instance B): %v", err)
	}
	if !delivered {
		t.Fatal("expected instance B to find the run via Redis and forward the cancel")
	}

	cmd := recvCommand(t, jobsA)
	if cmd.GetCancelJob() == nil || cmd.GetCancelJob().RunId != runID {
		t.Fatalf("expected the forwarded CancelJob to reach instance A's real worker connection, got %+v", cmd)
	}

	if _, err := redisClient.Get(ctx, runWorkerKey(runID)).Result(); err == nil {
		t.Error("expected the Redis run-worker mirror to be cleared once instance A's own cancelLocal handled the forward")
	}
}
