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

// KubernetesEngine is this codebase's sixth real engine - single-shot
// `kubectl apply` against a real, already-provisioned cluster. No
// `delete`/destroy-equivalent surfaced (matching every other engine's
// "apply only" reduction), and one cluster per Workspace, not
// multi-cluster or per-manifest targeting.
//
// Unlike every other engine so far, this one needs a real external
// credential (a kubeconfig) - deliberately per-workspace, not one
// shared static target the way docker-socket-proxy's DOCKER_HOST is for
// Compose/Packer (an operator-confirmed, real design choice). It
// arrives via JobAssignment.CredentialBundle, resolved from a second
// Variable (credentialVariableKeyByEngine, dispatch_run.go) the exact
// same live way ConfigBundle already is - reusing the existing
// Variables/Vault resolution path rather than inventing new plumbing.
type KubernetesEngine struct{}

func NewKubernetesEngine() *KubernetesEngine { return &KubernetesEngine{} }

func (e *KubernetesEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	if job.CredentialBundle == "" {
		// dispatch_run.go's own fail-fast on a missing
		// kubernetes_kubeconfig Variable should make this unreachable in
		// normal operation - this is defense in depth, not the primary
		// guard, so kubectl never falls through to a confusing error
		// against in-cluster/default config instead.
		return "", errors.New("kubernetes: no kubeconfig credential in job assignment")
	}

	workDir, err := os.MkdirTemp("", "job-"+job.RunId+"-kubernetes")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	kubeconfigPath := filepath.Join(workDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(job.CredentialBundle), 0o600); err != nil {
		return "", err
	}

	manifestPath := filepath.Join(workDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running kubectl apply", "run_id", job.RunId, "workdir", workDir)
	// kubectl auto-honors KUBECONFIG from the environment - the same
	// "no special flag-plumbing needed" property DOCKER_HOST already
	// relies on for Compose/Packer, just per-job here instead of
	// Worker-wide.
	cmd := exec.Command("kubectl", "apply", "-f", "manifest.yaml")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfigPath)
	out, err := execWithCancel(ctx, cmd, job.RunId, logger)
	if err != nil {
		return out, fmt.Errorf("kubectl apply: %w", err)
	}

	logger.Info("kubectl apply succeeded", "run_id", job.RunId)
	return out, nil
}
