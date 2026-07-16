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

const (
	kubesprayDir = "/opt/kubespray"
	// kubesprayAnsiblePlaybook is an absolute path, not a bare
	// "ansible-playbook" resolved off $PATH - Kubespray needs its own
	// dedicated venv (Dockerfile.worker), separate from
	// /opt/ansible-venv's own ansible-core 2.19.11 (AnsibleEngine),
	// since Kubespray's real requirements.txt pins the full "ansible"
	// community bundle (11.13.0, a materially different/larger package
	// than bare ansible-core) - confirmed against Kubespray's own
	// current requirements.txt, not assumed. Both venvs would have a
	// same-named "ansible-playbook" binary; resolving the bare name
	// would make this engine and AnsibleEngine race on PATH ordering
	// for which one actually runs. An absolute path sidesteps that
	// entirely rather than relying on PATH order to keep them apart.
	kubesprayAnsiblePlaybook = "/opt/kubespray-venv/bin/ansible-playbook"
)

// KubesprayEngine is this codebase's eighth and final real engine -
// single-shot `ansible-playbook cluster.yml` against Kubespray
// (github.com/kubernetes-sigs/kubespray, cloned at a pinned tag into
// the image at /opt/kubespray, Dockerfile.worker), real initial cluster
// bring-up on real target servers over SSH.
//
// Deliberately scoped to cluster.yml only (initial bring-up) - NOT
// scale.yml/upgrade-cluster.yml/remove-node.yml/reset.yml, and NOT the
// aspirational multi-Job-per-Run/phase model docs/architecture/
// 03-domain-model.md describes (that Job entity doesn't exist anywhere
// in this codebase - dispatch_run.go dispatches exactly one
// JobAssignment per Run today, the same single-shot-apply reduction
// every other engine already carries).
//
// ConfigBundle is the operator's real per-cluster Ansible inventory
// content (hosts.yaml format, with per-host vars like ansible_user
// directly in it, same "engine writes it verbatim, the real tool parses
// it" invariant as every other engine) - overlaid onto a copy of
// Kubespray's own inventory/sample (its real, working defaults, not
// fabricated). CredentialBundle is a real per-workspace SSH private key
// (same design as KubernetesEngine/HelmEngine's per-workspace
// kubeconfig - see KubernetesEngine's own doc comment) - written to a
// per-job temp file with 0600 permissions, which OpenSSH mandates, not
// just prefers.
type KubesprayEngine struct{}

func NewKubesprayEngine() *KubesprayEngine { return &KubesprayEngine{} }

func (e *KubesprayEngine) Execute(ctx context.Context, job *pb.JobAssignment, logger *slog.Logger) (string, error) {
	if job.CredentialBundle == "" {
		return "", errors.New("kubespray: no SSH private key credential in job assignment")
	}

	keyDir, err := os.MkdirTemp("", "job-"+job.RunId+"-kubespray-key")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(keyDir)

	keyPath := filepath.Join(keyDir, "id_rsa")
	if err := os.WriteFile(keyPath, []byte(job.CredentialBundle), 0o600); err != nil {
		return "", err
	}

	inventoryName := "job-" + job.RunId
	inventoryDir := filepath.Join(kubesprayDir, "inventory", inventoryName)
	defer func() {
		// Same forensic-log-then-cleanup pattern as terraformFamilyEngine
		// - a canceled or failed run's per-job inventory dir is the one
		// real trail left before it's discarded for good.
		if entries, readErr := os.ReadDir(inventoryDir); readErr == nil {
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			logger.Info("removing kubespray inventory dir", "run_id", job.RunId, "inventory_dir", inventoryDir, "files", names)
		}
		os.RemoveAll(inventoryDir)
	}()

	// Kubespray's own documented quickstart: copy inventory/sample
	// wholesale (real, working group_vars defaults, not fabricated),
	// then overwrite just hosts.yaml with the operator's real target
	// servers.
	copyCmd := exec.Command("cp", "-r", filepath.Join(kubesprayDir, "inventory", "sample"), inventoryDir)
	if out, err := copyCmd.CombinedOutput(); err != nil {
		return string(out), fmt.Errorf("kubespray: copy inventory/sample: %w", err)
	}

	hostsPath := filepath.Join(inventoryDir, "hosts.yaml")
	if err := os.WriteFile(hostsPath, []byte(job.ConfigBundle), 0o644); err != nil {
		return "", err
	}

	logger.Info("running kubespray cluster.yml", "run_id", job.RunId, "inventory_dir", inventoryDir)
	cmd := exec.Command(kubesprayAnsiblePlaybook,
		"-i", filepath.Join("inventory", inventoryName, "hosts.yaml"),
		"--private-key", keyPath,
		"-b",
		"cluster.yml",
	)
	cmd.Dir = kubesprayDir
	out, err := execWithCancel(ctx, cmd, job.RunId, logger)
	if err != nil {
		return out, fmt.Errorf("kubespray cluster.yml: %w", err)
	}

	logger.Info("kubespray cluster.yml succeeded", "run_id", job.RunId)
	return out, nil
}
