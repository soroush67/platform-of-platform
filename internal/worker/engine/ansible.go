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

// AnsibleEngine is this codebase's fifth real engine - single-shot
// playbook run against a local connection (`-i localhost, -c local`),
// deliberately no SSH/remote inventory in this slice (this codebase has
// no target-host/credential model yet - a real remote-inventory
// AnsibleEngine would need one, the same class of gap Kubespray/
// Kubernetes/Helm are deferred over). No init phase - a local-connection
// playbook using only Ansible's own built-in modules needs none;
// resolving external roles/collections via a requirements.yml is a
// real, separate addition not built here (deliberately scoped down,
// same posture as TerraformEngine's own documented reductions). No
// destroy-equivalent cleanup on cancel - Ansible has no symmetric
// "undo" any more than Terraform does.
type AnsibleEngine struct{}

func NewAnsibleEngine() *AnsibleEngine { return &AnsibleEngine{} }

func (e *AnsibleEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	workDir, err := os.MkdirTemp("", "job-"+job.RunId+"-ansible")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	playbookPath := filepath.Join(workDir, "playbook.yml")
	if err := os.WriteFile(playbookPath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running ansible-playbook", "run_id", job.RunId, "workdir", workDir)
	cmd := exec.Command("ansible-playbook", "-i", "localhost,", "-c", "local", "--diff", "playbook.yml")
	cmd.Dir = workDir
	out, err := execWithCancel(ctx, cmd, job.RunId, logger)
	if err != nil {
		return out, fmt.Errorf("ansible-playbook: %w", err)
	}

	logger.Info("ansible-playbook succeeded", "run_id", job.RunId)
	return out, nil
}
