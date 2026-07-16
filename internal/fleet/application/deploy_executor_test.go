package application_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

// ---- minimal in-memory fakes, scoped to exactly what DeployExecutor needs ----

type fakeMachineRepo struct {
	mu       sync.Mutex
	machines map[string]*domain.Machine // id -> machine
}

func newFakeMachineRepo() *fakeMachineRepo {
	return &fakeMachineRepo{machines: map[string]*domain.Machine{}}
}

func (f *fakeMachineRepo) Create(ctx context.Context, actorUserID string, m *domain.Machine) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.machines[m.ID] = m
	return nil
}
func (f *fakeMachineRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Machine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.machines[id]
	if !ok {
		return nil, domain.ErrMachineNotFound
	}
	return m, nil
}
func (f *fakeMachineRepo) ListByOrganization(ctx context.Context, organizationID string, includeArchived bool) ([]*domain.Machine, error) {
	return nil, nil
}
func (f *fakeMachineRepo) Update(ctx context.Context, actorUserID string, m *domain.Machine) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.machines[m.ID] = m
	return nil
}
func (f *fakeMachineRepo) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	return nil
}
func (f *fakeMachineRepo) Archive(ctx context.Context, actorUserID, organizationID, id string) error {
	return nil
}

type fakeComposeFileRepo struct {
	mu    sync.Mutex
	files map[string]*domain.ComposeFile
}

func newFakeComposeFileRepo() *fakeComposeFileRepo {
	return &fakeComposeFileRepo{files: map[string]*domain.ComposeFile{}}
}
func (f *fakeComposeFileRepo) Create(ctx context.Context, c *domain.ComposeFile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[c.ID] = c
	return nil
}
func (f *fakeComposeFileRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.ComposeFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.files[id]
	if !ok {
		return nil, domain.ErrComposeFileNotFound
	}
	return c, nil
}
func (f *fakeComposeFileRepo) GetGlobal(ctx context.Context, organizationID string) (*domain.ComposeFile, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.files {
		if c.OrganizationID == organizationID && c.IsGlobal {
			return c, true, nil
		}
	}
	return nil, false, nil
}
func (f *fakeComposeFileRepo) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.ComposeFile, error) {
	return nil, nil
}
func (f *fakeComposeFileRepo) UpdateContent(ctx context.Context, actorUserID, organizationID, id, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[id].ComposeContent = content
	return nil
}

type fakeVariableRepo struct {
	mu        sync.Mutex
	variables map[string][]*domain.Variable // composeFileID -> variables
}

func newFakeVariableRepo() *fakeVariableRepo {
	return &fakeVariableRepo{variables: map[string][]*domain.Variable{}}
}
func (f *fakeVariableRepo) Create(ctx context.Context, actorUserID string, v *domain.Variable) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.variables[v.ComposeFileID] = append(f.variables[v.ComposeFileID], v)
	return nil
}
func (f *fakeVariableRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Variable, error) {
	return nil, domain.ErrVariableNotFound
}
func (f *fakeVariableRepo) ListByComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Variable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.variables[composeFileID], nil
}
func (f *fakeVariableRepo) Update(ctx context.Context, actorUserID string, v *domain.Variable) error {
	return nil
}
func (f *fakeVariableRepo) Delete(ctx context.Context, actorUserID, organizationID, id string) error {
	return nil
}

type fakeAttachmentRepo struct{}

func (f *fakeAttachmentRepo) AttachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error {
	return nil
}
func (f *fakeAttachmentRepo) DetachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error {
	return nil
}
func (f *fakeAttachmentRepo) ListNetworksForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Network, error) {
	return nil, nil
}
func (f *fakeAttachmentRepo) AttachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID, containerPath string) error {
	return nil
}
func (f *fakeAttachmentRepo) DetachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID string) error {
	return nil
}
func (f *fakeAttachmentRepo) ListVolumesForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]application.VolumeAttachmentView, error) {
	return nil, nil
}

type fakeOperationRepo struct {
	mu         sync.Mutex
	operations map[string]*domain.Operation
}

func newFakeOperationRepo() *fakeOperationRepo {
	return &fakeOperationRepo{operations: map[string]*domain.Operation{}}
}
func (f *fakeOperationRepo) Create(ctx context.Context, o *domain.Operation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.operations[o.ID] = o
	return nil
}
func (f *fakeOperationRepo) GetByID(ctx context.Context, organizationID, id string) (*domain.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.operations[id]
	if !ok {
		return nil, domain.ErrOperationNotFound
	}
	cp := *o
	return &cp, nil
}
func (f *fakeOperationRepo) ListByOrganization(ctx context.Context, organizationID, composeFileID, machineID string) ([]*domain.Operation, error) {
	return nil, nil
}
func (f *fakeOperationRepo) TryClaim(ctx context.Context, organizationID, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.operations[id]
	if !ok || o.Status != domain.OperationStatusQueued {
		return false, nil
	}
	o.Status = domain.OperationStatusRunning
	return true, nil
}
func (f *fakeOperationRepo) MarkFinished(ctx context.Context, organizationID, id string, status domain.OperationStatus, exitCode *int, output string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.operations[id]
	if !ok {
		return domain.ErrOperationNotFound
	}
	o.Status = status
	o.ExitCode = exitCode
	o.Output = output
	return nil
}
func (f *fakeOperationRepo) get(id string) *domain.Operation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.operations[id]
}

type fakeScanner struct {
	repo *fakeOperationRepo
}

func (f *fakeScanner) FindQueuedCandidates(ctx context.Context, limit int) ([]application.OperationCandidate, error) {
	f.repo.mu.Lock()
	defer f.repo.mu.Unlock()
	var out []application.OperationCandidate
	for _, o := range f.repo.operations {
		if o.Status == domain.OperationStatusQueued {
			out = append(out, application.OperationCandidate{OperationID: o.ID, OrganizationID: o.OrganizationID})
		}
	}
	return out, nil
}

type fakeDeploySecretResolver struct {
	values map[string]string // "mountID|path" -> value
}

func (f *fakeDeploySecretResolver) ResolveValue(ctx context.Context, organizationID, mountID, path string) (string, error) {
	return f.values[mountID+"|"+path], nil
}

type fakeSSHRunner struct {
	mu           sync.Mutex
	runCalls     int
	lastCommand  string
	lastFiles    []application.RemoteFile
	exitCode     int
	output       string
	streamedLine string
}

func (f *fakeSSHRunner) Probe(ctx context.Context, target application.ConnectionTarget) (domain.ConnectionStatus, domain.DockerStatus, error) {
	return domain.ConnectionStatusOnline, domain.DockerStatusOK, nil
}
func (f *fakeSSHRunner) RunOperation(ctx context.Context, target application.ConnectionTarget, files []application.RemoteFile, command string, onLine func(string)) (int, string, error) {
	f.mu.Lock()
	f.runCalls++
	f.lastCommand = command
	f.lastFiles = files
	f.mu.Unlock()
	if onLine != nil {
		onLine(f.streamedLine)
	}
	return f.exitCode, f.output, nil
}

type fakeLogPublisher struct {
	mu    sync.Mutex
	lines []string
	ended bool
}

func (f *fakeLogPublisher) PublishLine(ctx context.Context, operationID, line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, line)
	return nil
}
func (f *fakeLogPublisher) PublishEnd(ctx context.Context, operationID string, exitCode int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended = true
	return nil
}

// ---- tests ----

func TestDeployExecutor_ClaimsExecutesAndPersistsSuccess(t *testing.T) {
	machines := newFakeMachineRepo()
	composeFiles := newFakeComposeFileRepo()
	variables := newFakeVariableRepo()
	operations := newFakeOperationRepo()
	attachments := &fakeAttachmentRepo{}
	secretResolver := &fakeDeploySecretResolver{values: map[string]string{"mount-1|ssh/key": "fake-private-key-material"}}
	sshRunner := &fakeSSHRunner{exitCode: 0, output: "Creating web... done\n", streamedLine: "Creating web... done"}
	publisher := &fakeLogPublisher{}

	machine, _ := domain.NewMachine("org-1", "m1", "10.0.0.5", 22, "deploy",
		domain.CredentialTypeSSHKey, domain.SecretReference{MountID: "mount-1", Path: "ssh/key"}, "/srv/deploy")
	machines.Create(context.Background(), "user-1", machine)

	composeFile, _ := domain.NewComposeFile("org-1", "web", "services:\n  web:\n    image: nginx\n", "user-1", false)
	composeFiles.Create(context.Background(), composeFile)

	op, _ := domain.NewOperation("org-1", composeFile.ID, machine.ID, domain.OperationTypeDeploy, "user-1")
	operations.Create(context.Background(), op)

	executor := application.NewDeployExecutor(
		&fakeScanner{repo: operations}, operations, machines, composeFiles, variables, attachments,
		secretResolver, sshRunner, publisher, 10*time.Millisecond, slog.New(slog.DiscardHandler),
	)

	// Run is the same Runnable shape outbox.Relay's own tests drive
	// directly (a real goroutine on a real ticker, not an unexported
	// pollOnce test hook) - a short poll interval keeps this fast.
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go executor.Run(runCtx)

	deadline := time.After(2 * time.Second)
	for {
		if op := operations.get(op.ID); op.Status == domain.OperationStatusSuccess || op.Status == domain.OperationStatusFailed {
			if op.Status != domain.OperationStatusSuccess {
				t.Fatalf("expected the operation to finish successfully, got status=%q output=%q", op.Status, op.Output)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the operation to finish")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if sshRunner.runCalls != 1 {
		t.Errorf("expected exactly one real SSH RunOperation call, got %d", sshRunner.runCalls)
	}
	if len(sshRunner.lastFiles) != 1 || sshRunner.lastFiles[0].Path == "" {
		t.Errorf("expected the rendered compose file to be written, got %+v", sshRunner.lastFiles)
	}
	if !publisher.ended {
		t.Error("expected PublishEnd to have been called")
	}
}

func TestDeployExecutor_ScrubsSecretValuesFromPublishedLines(t *testing.T) {
	machines := newFakeMachineRepo()
	composeFiles := newFakeComposeFileRepo()
	variables := newFakeVariableRepo()
	operations := newFakeOperationRepo()
	attachments := &fakeAttachmentRepo{}
	secretResolver := &fakeDeploySecretResolver{values: map[string]string{"mount-1|ssh/key": "fake-private-key-material", "mount-1|db/password": "super-secret-password"}}

	machine, _ := domain.NewMachine("org-1", "m1", "10.0.0.5", 22, "deploy",
		domain.CredentialTypeSSHKey, domain.SecretReference{MountID: "mount-1", Path: "ssh/key"}, "/srv/deploy")
	machines.Create(context.Background(), "user-1", machine)

	composeFile, _ := domain.NewComposeFile("org-1", "web", "services:\n  web:\n    image: nginx\n", "user-1", false)
	composeFiles.Create(context.Background(), composeFile)

	secretVar, _ := domain.NewVariableWithSecretRef("org-1", composeFile.ID, "DB_PASSWORD", "mount-1", "db/password")
	variables.Create(context.Background(), "user-1", secretVar)

	op, _ := domain.NewOperation("org-1", composeFile.ID, machine.ID, domain.OperationTypeUp, "user-1")
	operations.Create(context.Background(), op)

	sshRunner := &fakeSSHRunner{exitCode: 0, output: "connecting with super-secret-password now\n", streamedLine: "connecting with super-secret-password now"}
	publisher := &fakeLogPublisher{}

	executor := application.NewDeployExecutor(
		&fakeScanner{repo: operations}, operations, machines, composeFiles, variables, attachments,
		secretResolver, sshRunner, publisher, 10*time.Millisecond, slog.New(slog.DiscardHandler),
	)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go executor.Run(runCtx)

	deadline := time.After(2 * time.Second)
	for {
		if finished := operations.get(op.ID); finished.Status == domain.OperationStatusSuccess || finished.Status == domain.OperationStatusFailed {
			if strings.Contains(finished.Output, "super-secret-password") {
				t.Errorf("expected the persisted output to have the secret scrubbed, got %q", finished.Output)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the operation to finish")
		case <-time.After(10 * time.Millisecond):
		}
	}

	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	for _, line := range publisher.lines {
		if strings.Contains(line, "super-secret-password") {
			t.Errorf("expected every published log line to have the secret scrubbed, got %q", line)
		}
	}
}
