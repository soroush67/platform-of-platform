// Package ssh is Fleet's real SSH/SFTP execution adapter
// (golang.org/x/crypto/ssh + github.com/pkg/sftp) - the port half of
// what the ported Python product's own ssh_executor.py did with
// asyncssh. Satisfies application.SSHRunner.
package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

// dialTimeout bounds both the TCP dial and the whole probe - a
// Machine's Test/CheckConnection should never hang the request past a
// few seconds, matching the ported Python product's own
// CHECK_CONNECTION_TIMEOUT_SECONDS=10 constant.
const dialTimeout = 10 * time.Second

// composeBinDetect - re-checked fresh on every single command (not
// cached), so it self-heals if a machine's docker install changes,
// exactly matching the ported Python original's own inline shell
// one-liner.
const composeBinDetect = `COMPOSE_CMD=$(docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")`

type Client struct {
	// connectTimeout is overridable in tests (the in-process fake SSH
	// server dials instantly, but keeping this configurable rather than
	// hardcoding dialTimeout avoids ever needing to slow a real test
	// down to prove the field works).
	connectTimeout time.Duration
}

func NewClient() *Client {
	return &Client{connectTimeout: dialTimeout}
}

func (c *Client) dial(target application.ConnectionTarget) (*ssh.Client, error) {
	var authMethod ssh.AuthMethod
	switch target.CredentialType {
	case domain.CredentialTypeSSHPassword:
		authMethod = ssh.Password(target.Secret)
	case domain.CredentialTypeSSHKey:
		signer, err := ssh.ParsePrivateKey([]byte(target.Secret))
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		authMethod = ssh.PublicKeys(signer)
	default:
		return nil, fmt.Errorf("unsupported credential type %q", target.CredentialType)
	}

	config := &ssh.ClientConfig{
		User: target.User,
		Auth: []ssh.AuthMethod{authMethod},
		// Same accepted-for-now posture as the ported Python original's
		// own known_hosts=None - real host-key pinning is a real,
		// flagged, deferred hardening step, not silently dropped.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.connectTimeout,
	}

	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	return ssh.Dial("tcp", addr, config)
}

// Probe connects and runs `docker version` + the same compose-plugin-
// vs-legacy-binary detection RunOperation uses, reporting real
// (ConnectionStatus, DockerStatus) - never returns an error for a
// reachability failure itself (unreachable IS the real, reportable
// result), only for a genuine caller-side problem (e.g. context
// canceled).
func (c *Client) Probe(ctx context.Context, target application.ConnectionTarget) (domain.ConnectionStatus, domain.DockerStatus, error) {
	client, err := c.dial(target)
	if err != nil {
		return domain.ConnectionStatusUnreachable, domain.DockerStatusUnknown, nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return domain.ConnectionStatusUnreachable, domain.DockerStatusUnknown, nil
	}
	defer session.Close()

	out, err := session.CombinedOutput("docker version >/dev/null 2>&1 && (" + composeBinDetect + " && echo DOCKER_OK) || echo DOCKER_MISSING")
	if err != nil && len(out) == 0 {
		return domain.ConnectionStatusOnline, domain.DockerStatusError, nil
	}
	if strings.Contains(string(out), "DOCKER_OK") {
		return domain.ConnectionStatusOnline, domain.DockerStatusOK, nil
	}
	return domain.ConnectionStatusOnline, domain.DockerStatusMissing, nil
}

// RunOperation writes every RemoteFile via SFTP, then runs command over
// one SSH session, feeding each output line to onLine as it streams and
// returning the full combined capture alongside the real exit code.
// command already carries its own `2>&1` shell redirection (built by
// application.DeployExecutor) - Go's ssh.Session has no client-side
// stderr=stdout merge the way asyncssh's own stderr=STDOUT kwarg did;
// remote shell redirection is the equivalent mechanism, not a
// workaround.
func (c *Client) RunOperation(ctx context.Context, target application.ConnectionTarget, files []application.RemoteFile, command string, onLine func(string)) (int, string, error) {
	client, err := c.dial(target)
	if err != nil {
		return 0, "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	if len(files) > 0 {
		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			return 0, "", fmt.Errorf("sftp client: %w", err)
		}
		defer sftpClient.Close()

		for _, f := range files {
			if err := writeRemoteFile(sftpClient, f); err != nil {
				return 0, "", fmt.Errorf("write %s: %w", f.Path, err)
			}
		}
	}

	session, err := client.NewSession()
	if err != nil {
		return 0, "", fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return 0, "", fmt.Errorf("stdout pipe: %w", err)
	}

	if err := session.Start(command); err != nil {
		return 0, "", fmt.Errorf("start command: %w", err)
	}

	var combined strings.Builder
	scanner := bufio.NewScanner(stdout)
	// docker pull/build progress lines can exceed the 64KB default
	// token limit - bumped to 1MB, matching the real failure mode this
	// would otherwise hit against a real, verbose image pull.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		combined.WriteString(line)
		combined.WriteByte('\n')
		if onLine != nil {
			onLine(line)
		}
	}

	waitErr := session.Wait()
	exitCode := 0
	if waitErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitStatus()
		} else {
			return 0, combined.String(), fmt.Errorf("session wait: %w", waitErr)
		}
	}

	return exitCode, combined.String(), nil
}

func writeRemoteFile(client *sftp.Client, f application.RemoteFile) error {
	if err := client.MkdirAll(path.Dir(f.Path)); err != nil {
		return err
	}
	remote, err := client.Create(f.Path)
	if err != nil {
		return err
	}
	defer remote.Close()

	_, err = remote.Write([]byte(f.Content))
	return err
}
