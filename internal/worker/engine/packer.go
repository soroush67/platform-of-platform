package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

// PackerEngine is this codebase's sixth real engine - single-shot
// `packer init` + `packer build` against the Worker's own already-real
// DooD Docker access (docker-compose.yml's DOCKER_HOST pointing at
// docker-socket-proxy, the same path ComposeEngine already uses). A
// Docker-builder template's provisioners (e.g. shell) need real `docker
// exec` capability against the build container - docker-socket-proxy's
// EXEC category is deliberately enabled (EXEC: 1, docker-compose.yml)
// specifically for this, a real, operator-authorized narrowing of the
// isolation boundary that category previously closed entirely (see that
// service's own comment for the full reasoning and what's still denied).
// No destroy-equivalent cleanup on cancel or on success - a built image
// persisting after the Run is the actual point of an image-builder
// engine, not a gap to flag the way Terraform/Ansible's "no undo" is.
type PackerEngine struct{}

func NewPackerEngine() *PackerEngine { return &PackerEngine{} }

func (e *PackerEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	workDir, err := os.MkdirTemp("", "job-"+job.RunId+"-packer")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	templatePath := filepath.Join(workDir, "template.pkr.hcl")
	if err := os.WriteFile(templatePath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running packer init", "run_id", job.RunId, "workdir", workDir)
	initCmd := exec.Command("packer", "init", ".")
	initCmd.Dir = workDir
	initOut, err := execWithCancel(ctx, initCmd, job.RunId, logger)
	if err != nil {
		return initOut, fmt.Errorf("packer init: %w", err)
	}

	logger.Info("running packer build", "run_id", job.RunId, "workdir", workDir)
	// -force: a fresh throwaway workdir every run means Packer never
	// sees its own prior state, but the image *name* a template picks
	// (e.g. a fixed "packer-test:latest" tag used for real, repeatable
	// verification) can already exist in Docker from an earlier run -
	// -force allows Packer to overwrite/replace that same real tag
	// rather than erroring on the collision.
	buildCmd := exec.Command("packer", "build", "-force", ".")
	buildCmd.Dir = workDir
	buildOut, err := execWithCancel(ctx, buildCmd, job.RunId, logger)
	combined := initOut + "\n" + buildOut
	if err != nil {
		return combined, fmt.Errorf("packer build: %w", err)
	}

	logger.Info("packer build succeeded", "run_id", job.RunId)
	return combined, nil
}
