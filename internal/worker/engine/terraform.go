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

// TerraformEngine is this codebase's second real engine - deliberately
// scoped down (operator's own explicit choice this session, not an
// oversight): single-shot apply only (no separate Plan phase surfaced,
// matching ComposeEngine's own "no real plan concept" precedent),
// local-only providers expected (local_file/null_resource - no cloud
// credentials, JobAssignment carries none to inject), and NO persisted
// state across runs - every run does a fresh `terraform init` into a
// throwaway temp dir. That last point is a real, deliberate gap tied to
// this codebase's still-missing State/object-storage bounded context,
// not glossed over: a real Terraform Cloud-shaped product needs a
// persistent remote backend for `terraform apply` to be genuinely
// idempotent across runs of the same Workspace, which doesn't exist
// here yet.
//
// Licensing note, since this is a real, deliberate choice and not an
// oversight: Terraform >=1.6 is BSL 1.1, not MPL. This platform's own
// docs describe it as "a Terraform-Cloud-shaped product," and the
// ExecutionEngine enum already carries `opentofu` as a distinct value -
// a real signal the original design anticipated this exact tradeoff.
// OpenTofu (Apache-2.0, wire-compatible, same CLI flags, binary named
// `tofu`) is the lower-risk drop-in swap if BSL ever becomes
// unacceptable - not built here since the operator explicitly chose
// Terraform this session.
type TerraformEngine struct{}

func NewTerraformEngine() *TerraformEngine { return &TerraformEngine{} }

func (e *TerraformEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	workDir, err := os.MkdirTemp("", "job-"+job.RunId+"-tf")
	if err != nil {
		return "", err
	}
	defer func() {
		// No destroy-equivalent cleanup exists for Terraform in this
		// slice (unlike ComposeEngine's symmetric `down` on cancel) -
		// this is the one real forensic trail a canceled apply leaves
		// before its workdir (including any partial terraform.tfstate)
		// is discarded for good.
		if entries, readErr := os.ReadDir(workDir); readErr == nil {
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			logger.Info("removing terraform workdir", "run_id", job.RunId, "workdir", workDir, "files", names)
		}
		os.RemoveAll(workDir)
	}()

	configPath := filepath.Join(workDir, "main.tf")
	if err := os.WriteFile(configPath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running terraform init", "run_id", job.RunId, "workdir", workDir)
	initCmd := exec.Command("terraform", "init", "-input=false", "-no-color")
	initCmd.Dir = workDir
	initOut, err := execWithCancel(ctx, initCmd, job.RunId, logger)
	if err != nil {
		// %w, not %v - execWithCancel's ctx.Err() must survive this wrap
		// so handleJob's own errors.Is(err, context.Canceled) check
		// still sees through it; %v here would silently turn a canceled
		// run into a reported "failed" (the exact redundant-report bug
		// ComposeEngine's own cancellation path is designed to avoid).
		return initOut, fmt.Errorf("terraform init: %w", err)
	}

	logger.Info("running terraform apply", "run_id", job.RunId, "workdir", workDir)
	applyCmd := exec.Command("terraform", "apply", "-auto-approve", "-input=false", "-no-color")
	applyCmd.Dir = workDir
	applyOut, err := execWithCancel(ctx, applyCmd, job.RunId, logger)
	combined := initOut + "\n" + applyOut
	if err != nil {
		return combined, fmt.Errorf("terraform apply: %w", err)
	}

	logger.Info("terraform apply succeeded", "run_id", job.RunId)
	return combined, nil
}
