// The Worker binary (docs/architecture/17-workers.md §1) - maintains
// the long-lived gRPC connection to the Control Plane and, for each
// JobAssignment it receives, actually runs the work. This walking
// skeleton implements exactly one real engine: "compose" (docker
// compose up/down), reusing the same DooD (Docker-outside-of-Docker)
// pattern this operator's own compose-platform project already proved
// this session - real `docker compose` commands, not a simulated one.
//
// Deliberately not built here (documented gaps, not silent ones):
// the plugin-subprocess-over-Unix-socket second layer
// (docs/architecture/17-workers.md §2) - this Worker executes the
// compose engine in-process instead of launching a separate plugin
// binary; full per-job container isolation (§4) - this process reaches
// Docker through docker-socket-proxy (docker-compose.yml), not a raw
// mounted docker.sock, which blocks EXEC/BUILD/SWARM/SYSTEM/PLUGINS/
// SECRETS/NODES/SERVICES entirely, but the proxy scopes by API endpoint
// category, not by container ownership - one Job's compose still shares
// the same container namespace as every other Job's (no nested-dind-
// per-Job isolation, an explicit, discussed tradeoff, not an oversight);
// and the other seven engine types (terraform, opentofu, ansible, helm,
// packer, kubespray, kubernetes) - "compose" was chosen because it's
// the one this operator has a real, already-proven deployment pattern
// for from earlier this session.
package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
	"platform-of-platform/internal/platform/mtls"
	"platform-of-platform/internal/platform/tracing"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	controlPlaneAddr := getenvDefault("CONTROL_PLANE_GRPC_ADDR", "control-plane:9000")
	workerID := getenvDefault("WORKER_ID", "worker-1")
	tlsServerName := getenvDefault("TLS_SERVER_NAME", "control-plane")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	tracingShutdown, err := tracing.Setup(ctx, "worker")
	if err != nil {
		logger.Error("tracing setup failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tracingShutdown(context.Background()); err != nil {
			logger.Error("tracing shutdown failed", "error", err)
		}
	}()

	// Real mTLS (internal/platform/mtls) - this Worker presents its own
	// certificate (TLS_CLIENT_CERT/KEY, signed by the same dev CA the
	// Control Plane trusts) and verifies the Control Plane's server
	// certificate in turn. Replaces what used to be
	// insecure.NewCredentials() - a Worker without a cert signed by this
	// CA can no longer connect at all, not just "connects unencrypted."
	tlsCreds, err := mtls.ClientCredentials(
		getenvDefault("TLS_CA_CERT", "/certs/ca-cert.pem"),
		getenvDefault("TLS_CLIENT_CERT", "/certs/worker-cert.pem"),
		getenvDefault("TLS_CLIENT_KEY", "/certs/worker-key.pem"),
		tlsServerName,
	)
	if err != nil {
		logger.Error("mtls setup failed", "error", err)
		os.Exit(1)
	}
	// otelgrpc.NewClientHandler starts a real span for every RPC this
	// Worker makes (Register/StreamJobs/ReportJobStatus) and propagates
	// it over the wire via W3C tracecontext metadata - the Control
	// Plane's own otelgrpc server handler (cmd/control-plane/main.go)
	// continues it, making a request's actual HTTP->gRPC path visible as
	// one trace in Jaeger, not two disconnected per-process spans.
	conn, err := grpc.NewClient(controlPlaneAddr,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		logger.Error("failed to connect to control plane", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewWorkerServiceClient(conn)

	// running/runningMu are declared here, not inside runOnce, and
	// threaded through every runOnce call - they track this Worker's
	// actually-in-flight Jobs across stream reconnects, not just within
	// one connection's lifetime. A Job's own goroutine (handleJob) keeps
	// running through a broken stream regardless (its jobCtx derives
	// from the top-level ctx, not the stream's), but before this change
	// nothing else did: a reconnect used to start a brand new empty map,
	// silently losing the ability to route a later CancelJob to a Job
	// that was already in flight when the stream broke, and leaving
	// Register with no way to tell the Control Plane what it's still
	// actually running after a Control Plane restart wiped its Registry.
	var runningMu sync.Mutex
	running := make(map[string]*jobHandle)

	for {
		if err := runOnce(ctx, client, workerID, logger, &runningMu, running); err != nil {
			logger.Error("worker stream ended, reconnecting", "error", err)
		}
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// jobHandle is what lets a later CancelJob command actually reach a Job
// already running in its own goroutine - cancel just calls the stored
// context.CancelFunc, which handleJob's own select on ctx.Done() below
// is what turns into a real SIGTERM/SIGKILL to the subprocess.
type jobHandle struct {
	cancel context.CancelFunc
}

// runOnce processes commands from one StreamJobs connection until it
// breaks. JobAssignments are handled in their own goroutine (not inline
// in this receive loop) specifically so a CancelJob for an *earlier*
// Job can still be received and acted on while a later one is still
// running - a single sequential loop that called handleJob() directly
// couldn't call stream.Recv() again until the current Job finished,
// which would make Cancel commands impossible to deliver while
// anything was in flight.
func runOnce(ctx context.Context, client pb.WorkerServiceClient, workerID string, logger *slog.Logger, mu *sync.Mutex, running map[string]*jobHandle) error {
	mu.Lock()
	activeRunIDs := make([]string, 0, len(running))
	for runID := range running {
		activeRunIDs = append(activeRunIDs, runID)
	}
	mu.Unlock()

	regResp, err := client.Register(ctx, &pb.RegisterRequest{
		WorkerId:         workerID,
		SupportedEngines: []string{"compose"},
		Labels:           map[string]string{"region": "local"},
		ActiveRunIds:     activeRunIDs,
	})
	if err != nil {
		return err
	}
	logger.Info("registered with control plane", "worker_id", workerID, "accepted", regResp.Accepted, "active_run_ids", activeRunIDs)

	stream, err := client.StreamJobs(ctx, &pb.StreamJobsRequest{WorkerId: workerID})
	if err != nil {
		return err
	}

	for {
		cmd, err := stream.Recv()
		if err != nil {
			return err
		}

		switch c := cmd.Command.(type) {
		case *pb.WorkerCommand_JobAssignment:
			job := c.JobAssignment
			jobCtx, cancel := context.WithCancel(ctx)

			mu.Lock()
			running[job.RunId] = &jobHandle{cancel: cancel}
			mu.Unlock()

			logger.Info("received job assignment", "run_id", job.RunId, "engine", job.ExecutionEngine)
			go func() {
				defer func() {
					mu.Lock()
					delete(running, job.RunId)
					mu.Unlock()
					cancel() // release jobCtx's own resources either way
				}()
				handleJob(jobCtx, client, job, logger)
			}()

		case *pb.WorkerCommand_CancelJob:
			runID := c.CancelJob.RunId
			mu.Lock()
			h, ok := running[runID]
			mu.Unlock()
			if !ok {
				// Already finished, or this Worker was never the one
				// running it (a stale/duplicate route) - nothing to do,
				// not an error.
				logger.Info("received cancel for a run not active here, ignoring", "run_id", runID)
				continue
			}
			logger.Info("received cancel command", "run_id", runID)
			h.cancel()
		}
	}
}

// cancelGracePeriod - docs/architecture/17-workers.md §6: "waits a
// configurable grace period (default 30s), then SIGKILL if it hasn't
// exited." Shortened via env for real verification (waiting 30 real
// seconds every test run isn't practical), matching the same pattern
// already used for the Stale Run Reaper's own timings.
func cancelGracePeriod() time.Duration {
	if v := os.Getenv("CANCEL_GRACE_PERIOD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 30 * time.Second
}

func handleJob(ctx context.Context, client pb.WorkerServiceClient, job *pb.JobAssignment, logger *slog.Logger) {
	if job.ExecutionEngine != "compose" {
		report(ctx, client, job, "failed", "", "unsupported execution engine: "+job.ExecutionEngine+" (only compose is implemented)")
		return
	}

	tmpFile, err := os.CreateTemp("", "job-*.compose.yml")
	if err != nil {
		report(ctx, client, job, "failed", "", "failed to create temp compose file: "+err.Error())
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(job.ConfigBundle); err != nil {
		tmpFile.Close()
		report(ctx, client, job, "failed", "", "failed to write compose file: "+err.Error())
		return
	}
	tmpFile.Close()

	projectName := "run-" + job.RunId

	// docker-compose (hyphenated v2 standalone binary), not `docker
	// compose` (the plugin subcommand) - the plugin isn't installed in
	// this environment (verified for real: `docker compose` errored
	// with "unknown shorthand flag" since docker.io doesn't ship it),
	// the standalone binary is what's actually available.
	var out bytes.Buffer
	cmd := exec.Command("docker-compose", "-f", tmpFile.Name(), "-p", projectName, "up", "-d")
	cmd.Stdout = &out
	cmd.Stderr = &out
	// Its own process group (docs/architecture/17-workers.md §6: "SIGTERM
	// to the plugin subprocess's entire process group... a terraform
	// apply spawns provider subprocesses of its own that need to receive
	// the signal too" - docker-compose can equally spawn its own
	// children) - plain exec.CommandContext only ever SIGKILLs the
	// immediate process on ctx cancellation, with no grace period and no
	// process-group reach, so this Job manages its own subprocess
	// lifecycle explicitly instead.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logger.Info("running docker compose up", "run_id", job.RunId, "project", projectName)
	if err := cmd.Start(); err != nil {
		report(ctx, client, job, "failed", "", "failed to start docker-compose: "+err.Error())
		return
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case runErr := <-done:
		if runErr != nil {
			logger.Error("docker compose up failed", "run_id", job.RunId, "error", runErr, "output", out.String())
			report(ctx, client, job, "failed", out.String(), runErr.Error())
			return
		}
		logger.Info("docker compose up succeeded", "run_id", job.RunId, "output", out.String())
		report(ctx, client, job, "applied", out.String(), "")

	case <-ctx.Done():
		// Real cancellation - docs/architecture/17-workers.md §6's exact
		// sequence: SIGTERM the whole process group, wait a grace
		// period, SIGKILL if it hasn't exited by then.
		logger.Info("run canceled, sending SIGTERM to process group", "run_id", job.RunId, "pgid", cmd.Process.Pid)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

		select {
		case <-done:
		case <-time.After(cancelGracePeriod()):
			logger.Info("grace period expired, sending SIGKILL", "run_id", job.RunId)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}

		// Best-effort cleanup of any partially-created resources
		// (docs/architecture/17-workers.md §6: "the container itself is
		// destroyed regardless of clean-exit success") - a compose file
		// with more than one service could have had some, not all, of
		// its containers created before the kill landed.
		cleanup := exec.Command("docker-compose", "-f", tmpFile.Name(), "-p", projectName, "down")
		if out, err := cleanup.CombinedOutput(); err != nil {
			logger.Error("post-cancel cleanup failed", "run_id", job.RunId, "error", err, "output", string(out))
		}

		// No ReportJobStatus call here - CancelRunService already made
		// the Run's `canceled` transition synchronously, in the same
		// request that sent this Worker its CancelJob command
		// (cmd/control-plane's cancel_run.go). This Job's only
		// remaining responsibility was to make the real subprocess
		// actually stop, not to report a second, redundant status.
		logger.Info("run canceled and cleaned up", "run_id", job.RunId)
	}
}

func report(ctx context.Context, client pb.WorkerServiceClient, job *pb.JobAssignment, status, logLine, errMsg string) {
	_, err := client.ReportJobStatus(ctx, &pb.JobStatusReport{
		RunId:          job.RunId,
		OrganizationId: job.OrganizationId,
		WorkspaceId:    job.WorkspaceId,
		Status:         status,
		LogLine:        logLine,
		ErrorMessage:   errMsg,
	})
	if err != nil {
		// Nothing more this Worker can do about a failed status report
		// short of retrying - a real gap (no report-retry logic here),
		// flagged rather than silently swallowed.
		slog.Default().Error("failed to report job status", "run_id", job.RunId, "error", err)
	}
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
