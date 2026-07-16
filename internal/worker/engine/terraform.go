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

// terraformFamilyEngine is shared by TerraformEngine and OpenTofuEngine -
// Terraform and OpenTofu are wire/CLI-compatible forks (same HCL, same
// init/apply flags), so the only real difference between the two engines
// is which binary gets invoked. Extracting this now (rather than
// copy-pasting TerraformEngine's own body for OpenTofuEngine) means a
// change to this shared behavior only needs to happen once.
//
// Deliberately scoped down (operator's own explicit choice this
// session, not an oversight): single-shot apply only (no separate Plan
// phase surfaced, matching ComposeEngine's own "no real plan concept"
// precedent), local-only providers expected (local_file/null_resource -
// no cloud credentials, JobAssignment carries none to inject), and NO
// persisted state across runs - every run does a fresh `init` into a
// throwaway temp dir. That last point is a real, deliberate gap tied to
// this codebase's still-missing State/object-storage bounded context,
// not glossed over: a real Terraform Cloud-shaped product needs a
// persistent remote backend for `apply` to be genuinely idempotent
// across runs of the same Workspace, which doesn't exist here yet.
type terraformFamilyEngine struct {
	binary string
}

// TerraformEngine is this codebase's second real engine.
//
// Licensing note, since this is a real, deliberate choice and not an
// oversight: Terraform >=1.6 is BSL 1.1, not MPL. This platform's own
// docs describe it as "a Terraform-Cloud-shaped product," and the
// ExecutionEngine enum already carries `opentofu` as a distinct value -
// a real signal the original design anticipated this exact tradeoff.
// OpenTofuEngine (opentofu.go, Apache-2.0, wire-compatible, same CLI
// flags, binary named `tofu`) is the lower-risk alternative, built
// alongside this one rather than as a silent swap.
type TerraformEngine struct{ terraformFamilyEngine }

func NewTerraformEngine() *TerraformEngine {
	return &TerraformEngine{terraformFamilyEngine{binary: "terraform"}}
}

func (e *terraformFamilyEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	workDir, err := os.MkdirTemp("", "job-"+job.RunId+"-"+e.binary)
	if err != nil {
		return "", err
	}
	defer func() {
		// No destroy-equivalent cleanup exists for this engine family in
		// this slice (unlike ComposeEngine's symmetric `down` on cancel) -
		// this is the one real forensic trail a canceled apply leaves
		// before its workdir (including any partial *.tfstate) is
		// discarded for good.
		if entries, readErr := os.ReadDir(workDir); readErr == nil {
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			logger.Info("removing "+e.binary+" workdir", "run_id", job.RunId, "workdir", workDir, "files", names)
		}
		os.RemoveAll(workDir)
	}()

	configPath := filepath.Join(workDir, "main.tf")
	if err := os.WriteFile(configPath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running "+e.binary+" init", "run_id", job.RunId, "workdir", workDir)
	initCmd := exec.Command(e.binary, "init", "-input=false", "-no-color")
	initCmd.Dir = workDir
	initOut, err := execWithCancel(ctx, initCmd, job.RunId, logger)
	if err != nil {
		// %w, not %v - execWithCancel's ctx.Err() must survive this wrap
		// so handleJob's own errors.Is(err, context.Canceled) check
		// still sees through it; %v here would silently turn a canceled
		// run into a reported "failed" (the exact redundant-report bug
		// ComposeEngine's own cancellation path is designed to avoid).
		return initOut, fmt.Errorf("%s init: %w", e.binary, err)
	}

	logger.Info("running "+e.binary+" apply", "run_id", job.RunId, "workdir", workDir)
	applyCmd := exec.Command(e.binary, "apply", "-auto-approve", "-input=false", "-no-color")
	applyCmd.Dir = workDir
	applyOut, err := execWithCancel(ctx, applyCmd, job.RunId, logger)
	combined := initOut + "\n" + applyOut
	if err != nil {
		return combined, fmt.Errorf("%s apply: %w", e.binary, err)
	}

	logger.Info(e.binary+" apply succeeded", "run_id", job.RunId)
	return combined, nil
}
