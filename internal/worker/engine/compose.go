package engine

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

// ComposeEngine is this codebase's one already-proven real engine
// (docker compose up/down), reusing the same DooD pattern this
// operator's own compose-platform project already proved earlier this
// session - moved here verbatim from cmd/worker/main.go's own
// (previously monolithic) handleJob, zero behavior change.
type ComposeEngine struct{}

func NewComposeEngine() *ComposeEngine { return &ComposeEngine{} }

func (e *ComposeEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	tmpFile, err := os.CreateTemp("", "job-*.compose.yml")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(job.ConfigBundle); err != nil {
		tmpFile.Close()
		return "", err
	}
	tmpFile.Close()

	projectName := "run-" + job.RunId

	// docker-compose (hyphenated v2 standalone binary), not `docker
	// compose` (the plugin subcommand) - the plugin isn't installed in
	// this environment (verified for real: `docker compose` errored
	// with "unknown shorthand flag" since docker.io doesn't ship it),
	// the standalone binary is what's actually available.
	logger.Info("running docker compose up", "run_id", job.RunId, "project", projectName)
	cmd := exec.Command("docker-compose", "-f", tmpFile.Name(), "-p", projectName, "up", "-d")
	out, err := execWithCancel(ctx, cmd, job.RunId, logger)

	if errors.Is(err, context.Canceled) {
		// Best-effort cleanup of any partially-created resources
		// (docs/architecture/17-workers.md §6: "the container itself is
		// destroyed regardless of clean-exit success") - a compose file
		// with more than one service could have had some, not all, of
		// its containers created before the kill landed.
		cleanup := exec.Command("docker-compose", "-f", tmpFile.Name(), "-p", projectName, "down")
		if cleanupOut, cleanupErr := cleanup.CombinedOutput(); cleanupErr != nil {
			logger.Error("post-cancel cleanup failed", "run_id", job.RunId, "error", cleanupErr, "output", string(cleanupOut))
		}
		logger.Info("run canceled and cleaned up", "run_id", job.RunId)
		return out, err
	}

	if err != nil {
		logger.Error("docker compose up failed", "run_id", job.RunId, "error", err, "output", out)
		return out, err
	}

	logger.Info("docker compose up succeeded", "run_id", job.RunId, "output", out)
	return out, nil
}
