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
// binary; per-job container isolation (§4) - this process itself runs
// with docker.sock access, not each Job in its own sandboxed container;
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	controlPlaneAddr := getenvDefault("CONTROL_PLANE_GRPC_ADDR", "control-plane:9000")
	workerID := getenvDefault("WORKER_ID", "worker-1")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Insecure transport credentials - dev-only, same posture as this
	// codebase's other bootstrap-secret shortcuts (JWT_SIGNING_KEY,
	// CockroachDB --insecure) - a real deployment needs mTLS here
	// (docs/architecture/17-workers.md's own worker identity token),
	// not built in this walking skeleton.
	conn, err := grpc.NewClient(controlPlaneAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("failed to connect to control plane", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewWorkerServiceClient(conn)

	for {
		if err := runOnce(ctx, client, workerID, logger); err != nil {
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
func runOnce(ctx context.Context, client pb.WorkerServiceClient, workerID string, logger *slog.Logger) error {
	regResp, err := client.Register(ctx, &pb.RegisterRequest{
		WorkerId:         workerID,
		SupportedEngines: []string{"compose"},
		Labels:           map[string]string{"region": "local"},
	})
	if err != nil {
		return err
	}
	logger.Info("registered with control plane", "worker_id", workerID, "accepted", regResp.Accepted)

	stream, err := client.StreamJobs(ctx, &pb.StreamJobsRequest{WorkerId: workerID})
	if err != nil {
		return err
	}

	var mu sync.Mutex
	running := make(map[string]*jobHandle)

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
