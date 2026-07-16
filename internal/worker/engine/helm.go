package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

// HelmEngine is this codebase's seventh real engine - single-shot
// `helmfile apply`. A real Helm chart is a directory (Chart.yaml +
// templates/), not one file, so unlike every other engine's ConfigBundle
// (which a single real binary parses whole), a bare chart couldn't be
// ConfigBundle's own content the way main.tf/playbook.yml already are.
// Rather than inventing Go-side parsing to pull a chart/release/
// namespace out of one string (the first precedent break in this
// package - every other engine writes ConfigBundle verbatim and lets
// the real binary do 100% of the parsing), this installs Helmfile
// (github.com/helmfile/helmfile, a real, separate static binary,
// Dockerfile.worker) as a sixth tool: ConfigBundle is a complete
// `helmfile.yaml` (Helmfile's own real schema for chart+release+
// namespace+values in one file), written verbatim like every other
// engine. Zero parsing code here.
//
// Same per-workspace kubeconfig-via-CredentialBundle handling as
// KubernetesEngine - see that type's own doc comment for the real
// design reasoning (operator-confirmed per-workspace, not shared
// static).
type HelmEngine struct{}

func NewHelmEngine() *HelmEngine { return &HelmEngine{} }

func (e *HelmEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	if job.CredentialBundle == "" {
		return "", errors.New("helm: no kubeconfig credential in job assignment")
	}

	workDir, err := os.MkdirTemp("", "job-"+job.RunId+"-helm")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	kubeconfigPath := filepath.Join(workDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(job.CredentialBundle), 0o600); err != nil {
		return "", err
	}

	helmfilePath := filepath.Join(workDir, "helmfile.yaml")
	if err := os.WriteFile(helmfilePath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running helmfile apply", "run_id", job.RunId, "workdir", workDir)
	cmd := exec.Command("helmfile", "apply")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfigPath)
	out, err := execWithCancel(ctx, cmd, job.RunId, logger)
	if err != nil {
		return out, fmt.Errorf("helmfile apply: %w", err)
	}

	logger.Info("helmfile apply succeeded", "run_id", job.RunId)
	return out, nil
}
