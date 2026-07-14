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

	for {
		job, err := stream.Recv()
		if err != nil {
			return err
		}
		logger.Info("received job assignment", "run_id", job.RunId, "engine", job.ExecutionEngine)
		handleJob(ctx, client, job, logger)
	}
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
	var out bytes.Buffer
	// docker-compose (hyphenated v2 standalone binary), not `docker
	// compose` (the plugin subcommand) - the plugin isn't installed in
	// this environment (verified for real: `docker compose` errored
	// with "unknown shorthand flag" since docker.io doesn't ship it),
	// the standalone binary is what's actually available.
	cmd := exec.CommandContext(ctx, "docker-compose", "-f", tmpFile.Name(), "-p", projectName, "up", "-d")
	cmd.Stdout = &out
	cmd.Stderr = &out

	logger.Info("running docker compose up", "run_id", job.RunId, "project", projectName)
	runErr := cmd.Run()

	if runErr != nil {
		logger.Error("docker compose up failed", "run_id", job.RunId, "error", runErr, "output", out.String())
		report(ctx, client, job, "failed", out.String(), runErr.Error())
		return
	}

	logger.Info("docker compose up succeeded", "run_id", job.RunId, "output", out.String())
	report(ctx, client, job, "applied", out.String(), "")
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
