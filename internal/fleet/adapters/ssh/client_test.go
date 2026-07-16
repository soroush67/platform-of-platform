package ssh_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	xssh "golang.org/x/crypto/ssh"

	"github.com/pkg/sftp"

	"platform-of-platform/internal/fleet/adapters/ssh"
	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

func mustGenerateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return key
}

const testPassword = "test-password"

// fakeSSHServer is an in-process real SSH server
// (golang.org/x/crypto/ssh's own server side over a real localhost TCP
// listener) - exercises Client's real dial/auth/session/SFTP/exec code
// path end to end with zero external infra, per the Fleet plan's own
// verification step 7. onExec decides what a real "exec" request
// returns (output + exit code); the SFTP subsystem is always backed by
// sftp.InMemHandler() so file-write tests can assert against it.
type fakeSSHServer struct {
	addr    string
	onExec  func(command string) (output string, exitCode uint32)
	handler sftp.Handlers
}

func newFakeSSHServer(t *testing.T, onExec func(command string) (string, uint32)) *fakeSSHServer {
	t.Helper()

	hostKey, err := xssh.NewSignerFromKey(mustGenerateRSAKey(t))
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}

	config := &xssh.ServerConfig{
		PasswordCallback: func(conn xssh.ConnMetadata, password []byte) (*xssh.Permissions, error) {
			if string(password) != testPassword {
				return nil, fmt.Errorf("wrong password")
			}
			return nil, nil
		},
	}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	srv := &fakeSSHServer{addr: listener.Addr().String(), onExec: onExec, handler: sftp.InMemHandler()}

	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			go srv.handleConn(t, nConn, config)
		}
	}()

	return srv
}

func (s *fakeSSHServer) handleConn(t *testing.T, nConn net.Conn, config *xssh.ServerConfig) {
	sshConn, chans, reqs, err := xssh.NewServerConn(nConn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go xssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(xssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go s.handleSession(channel, requests)
	}
}

func (s *fakeSSHServer) handleSession(channel xssh.Channel, requests <-chan *xssh.Request) {
	for req := range requests {
		switch req.Type {
		case "exec":
			var payload struct{ Command string }
			xssh.Unmarshal(req.Payload, &payload)
			req.Reply(true, nil)

			output, exitCode := s.onExec(payload.Command)
			io.WriteString(channel, output)
			channel.SendRequest("exit-status", false, xssh.Marshal(struct{ Status uint32 }{exitCode}))
			channel.Close()
		case "subsystem":
			var payload struct{ Name string }
			xssh.Unmarshal(req.Payload, &payload)
			if payload.Name != "sftp" {
				req.Reply(false, nil)
				continue
			}
			req.Reply(true, nil)
			server := sftp.NewRequestServer(channel, s.handler)
			server.Serve()
			channel.Close()
		default:
			req.Reply(false, nil)
		}
	}
}

func (s *fakeSSHServer) target(credentialType domain.CredentialType, secret string) application.ConnectionTarget {
	host, portStr, _ := net.SplitHostPort(s.addr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	return application.ConnectionTarget{Host: host, Port: port, User: "test-user", CredentialType: credentialType, Secret: secret}
}

func TestClient_Probe_ReportsOnlineAndDockerOK(t *testing.T) {
	srv := newFakeSSHServer(t, func(command string) (string, uint32) {
		return "DOCKER_OK\n", 0
	})
	client := ssh.NewClient()

	connStatus, dockerStatus, err := client.Probe(context.Background(), srv.target(domain.CredentialTypeSSHPassword, testPassword))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if connStatus != domain.ConnectionStatusOnline {
		t.Errorf("expected ConnectionStatusOnline, got %q", connStatus)
	}
	if dockerStatus != domain.DockerStatusOK {
		t.Errorf("expected DockerStatusOK, got %q", dockerStatus)
	}
}

func TestClient_Probe_UnreachableOnBadAuth(t *testing.T) {
	srv := newFakeSSHServer(t, func(command string) (string, uint32) { return "DOCKER_OK\n", 0 })
	client := ssh.NewClient()

	connStatus, dockerStatus, err := client.Probe(context.Background(), srv.target(domain.CredentialTypeSSHPassword, "wrong-password"))
	if err != nil {
		t.Fatalf("expected Probe to report unreachable, not return an error: %v", err)
	}
	if connStatus != domain.ConnectionStatusUnreachable {
		t.Errorf("expected ConnectionStatusUnreachable for bad auth, got %q", connStatus)
	}
	if dockerStatus != domain.DockerStatusUnknown {
		t.Errorf("expected DockerStatusUnknown for bad auth, got %q", dockerStatus)
	}
}

func TestClient_RunOperation_CapturesOutputExitCodeAndStreamsLines(t *testing.T) {
	srv := newFakeSSHServer(t, func(command string) (string, uint32) {
		if !strings.Contains(command, "docker-compose") && !strings.Contains(command, "docker compose") {
			t.Errorf("expected the command to reference compose, got %q", command)
		}
		return "line one\nline two\n", 1
	})
	client := ssh.NewClient()

	var streamed []string
	exitCode, output, err := client.RunOperation(context.Background(), srv.target(domain.CredentialTypeSSHPassword, testPassword), nil,
		"COMPOSE_CMD=$(docker compose version || echo docker-compose) && $COMPOSE_CMD up -d 2>&1",
		func(line string) { streamed = append(streamed, line) },
	)
	if err != nil {
		t.Fatalf("RunOperation: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(output, "line one") || !strings.Contains(output, "line two") {
		t.Errorf("expected both lines in the captured output, got %q", output)
	}
	if len(streamed) != 2 || streamed[0] != "line one" || streamed[1] != "line two" {
		t.Errorf("expected onLine to be called once per line in order, got %v", streamed)
	}
}

func TestClient_RunOperation_WritesFilesViaSFTP(t *testing.T) {
	srv := newFakeSSHServer(t, func(command string) (string, uint32) { return "ok\n", 0 })
	client := ssh.NewClient()
	target := srv.target(domain.CredentialTypeSSHPassword, testPassword)

	files := []application.RemoteFile{{Path: "/srv/deploy/app/docker-compose.yml", Content: "services:\n  web:\n    image: nginx\n"}}
	_, _, err := client.RunOperation(context.Background(), target, files, "true", nil)
	if err != nil {
		t.Fatalf("RunOperation: %v", err)
	}

	// Read back through a fresh, real SFTP client session against the
	// same in-memory-backed fake server, reusing real pkg/sftp client
	// code (not a hand-built sftp.Request) - proves the write really
	// happened, not just that RunOperation returned no error.
	conn, err := xssh.Dial("tcp", srv.addr, &xssh.ClientConfig{
		User:            target.User,
		Auth:            []xssh.AuthMethod{xssh.Password(testPassword)},
		HostKeyCallback: xssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("dial for readback: %v", err)
	}
	defer conn.Close()
	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		t.Fatalf("sftp.NewClient for readback: %v", err)
	}
	defer sftpClient.Close()

	f, err := sftpClient.Open("/srv/deploy/app/docker-compose.yml")
	if err != nil {
		t.Fatalf("open written file for readback: %v", err)
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(content) != files[0].Content {
		t.Errorf("expected the SFTP-written file content to round-trip, got %q", string(content))
	}
}
