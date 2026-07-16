// Package engine is the Worker's real, minimal in-process engine
// abstraction (docs/architecture/17-workers.md §1's own "the Worker
// itself never runs Terraform/Ansible/Helm code directly - it's a
// supervisor" describes a much heavier plugin-subprocess-over-Unix-socket
// design, §2 - none of that exists here; this package is the honestly-
// reduced, in-process substitute cmd/worker's own top comment already
// flags). Not structured as domain/application/adapters like every
// Control-Plane bounded context - the Worker isn't a persistence-backed
// context, it's a stateless job executor, so a flat package is the right
// shape (same posture as internal/platform/mtls, internal/platform/
// tracing).
package engine

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

// Engine is what cmd/worker's handleJob dispatches a JobAssignment to,
// keyed by ExecutionEngine string in a plain map - adding a third engine
// later means writing one new type satisfying this interface and adding
// one map entry, not touching handleJob's own dispatch logic at all.
type Engine interface {
	// Execute runs job to completion or until ctx is canceled, returning
	// captured output either way. If ctx is canceled before the
	// underlying process exits, Execute is responsible for terminating
	// it and any engine-specific cleanup, then returning an error for
	// which errors.Is(err, context.Canceled) is true - handleJob uses
	// that to skip the redundant ReportJobStatus call (CancelRunService
	// already transitioned the Run server-side by the time a Job's own
	// context gets canceled).
	Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (output string, err error)
}

// cancelGracePeriod - docs/architecture/17-workers.md §6: "waits a
// configurable grace period (default 30s), then SIGKILL if it hasn't
// exited." Shortened via env for real verification (waiting 30 real
// seconds every test run isn't practical), matching the same pattern
// already used for the Stale Run Reaper's own timings. Shared by every
// Engine via execWithCancel below - one cancellation policy, not one per
// engine.
func cancelGracePeriod() time.Duration {
	if v := os.Getenv("CANCEL_GRACE_PERIOD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 30 * time.Second
}

// execWithCancel runs cmd to completion in its own process group,
// returning combined stdout+stderr - it owns and wires up cmd.Stdout/
// cmd.Stderr itself (callers must not set them) so there's exactly one
// place in this package that can silently drop output by racing two
// writers onto the same *bytes.Buffer. On ctx cancellation it sends
// SIGTERM to the whole process group (docs/architecture/17-workers.md
// §6: "a terraform apply spawns provider subprocesses of its own that
// need to receive the signal too" - the exact reason a process group,
// not just the immediate child, is targeted), waits cancelGracePeriod,
// then SIGKILL - shared by every Engine so this dance is written once,
// not once per engine.
func execWithCancel(ctx context.Context, cmd *exec.Cmd, runID string, logger *slog.Logger) (string, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return out.String(), err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case runErr := <-done:
		return out.String(), runErr

	case <-ctx.Done():
		logger.Info("run canceled, sending SIGTERM to process group", "run_id", runID, "pgid", cmd.Process.Pid)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

		select {
		case <-done:
		case <-time.After(cancelGracePeriod()):
			logger.Info("grace period expired, sending SIGKILL", "run_id", runID)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		return out.String(), ctx.Err()
	}
}
