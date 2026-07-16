package application

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"

	"platform-of-platform/internal/fleet/domain"
)

// DeployExecutor is Fleet's own Runnable (internal/platform/outbox.
// Relay's own ticker-poll-loop shape, started as one more g.Go(...)
// line in main.go's existing errgroup) - real SSH deploy execution runs
// in-process here, not via the gRPC Worker/Job/Run model (see the Fleet
// plan's own decision #3 for why: the Worker model is Workspace/Run-
// centric, it doesn't fit "a reusable Machine/ComposeFile catalog,
// deploy on demand to any of N machines").
type DeployExecutor struct {
	scanner        QueuedOperationScanner
	operations     OperationRepository
	machines       MachineRepository
	composeFiles   ComposeFileRepository
	variables      VariableRepository
	attachments    AttachmentRepository
	secretResolver SecretResolver
	ssh            SSHRunner
	publisher      LogPublisher
	pollInterval   time.Duration
	batchSize      int
	logger         *slog.Logger
}

func NewDeployExecutor(
	scanner QueuedOperationScanner,
	operations OperationRepository,
	machines MachineRepository,
	composeFiles ComposeFileRepository,
	variables VariableRepository,
	attachments AttachmentRepository,
	secretResolver SecretResolver,
	sshRunner SSHRunner,
	publisher LogPublisher,
	pollInterval time.Duration,
	logger *slog.Logger,
) *DeployExecutor {
	return &DeployExecutor{
		scanner: scanner, operations: operations, machines: machines, composeFiles: composeFiles,
		variables: variables, attachments: attachments, secretResolver: secretResolver,
		ssh: sshRunner, publisher: publisher, pollInterval: pollInterval, batchSize: 20, logger: logger,
	}
}

func (e *DeployExecutor) Run(ctx context.Context) error {
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := e.pollOnce(ctx); err != nil {
				e.logger.Error("fleet deploy executor poll failed", "error", err)
			}
		}
	}
}

func (e *DeployExecutor) pollOnce(ctx context.Context) error {
	candidates, err := e.scanner.FindQueuedCandidates(ctx, e.batchSize)
	if err != nil {
		return err
	}

	for _, c := range candidates {
		claimed, err := e.operations.TryClaim(ctx, c.OrganizationID, c.OperationID)
		if err != nil {
			e.logger.Error("failed to claim operation", "operation_id", c.OperationID, "error", err)
			continue
		}
		if !claimed {
			// Already claimed by an earlier poll tick (or, once this
			// codebase runs multiple control-plane replicas, a
			// different one) - not an error, same "compare-and-swap
			// already lost, move on" posture as Execution's own
			// TryStartApplying callers.
			continue
		}

		// A slow deploy shouldn't block the next poll tick from
		// claiming/starting others.
		go e.execute(ctx, c.OrganizationID, c.OperationID)
	}

	return nil
}

func (e *DeployExecutor) execute(ctx context.Context, organizationID, operationID string) {
	operation, err := e.operations.GetByID(ctx, organizationID, operationID)
	if err != nil {
		e.logger.Error("failed to load claimed operation", "operation_id", operationID, "error", err)
		return
	}

	composeFile, err := e.composeFiles.GetByID(ctx, organizationID, operation.ComposeFileID)
	if err != nil {
		e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("load compose file: %w", err))
		return
	}
	machine, err := e.machines.GetByID(ctx, organizationID, operation.MachineID)
	if err != nil {
		e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("load machine: %w", err))
		return
	}

	secret, err := e.secretResolver.ResolveValue(ctx, organizationID, machine.CredentialRef.MountID, machine.CredentialRef.Path)
	if err != nil {
		e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("resolve machine credential: %w", err))
		return
	}

	resolved, err := resolveComposeVariables(ctx, organizationID, operation.ComposeFileID, e.variables, e.composeFiles, e.secretResolver)
	if err != nil {
		e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("resolve variables: %w", err))
		return
	}
	secretValues := secretValuesFor(resolved)

	deployPath := path.Join(machine.DeployBasePath, slugify(composeFile.Name))

	var files []RemoteFile
	if operation.OperationType == domain.OperationTypeDeploy {
		networks, err := e.attachments.ListNetworksForComposeFile(ctx, organizationID, operation.ComposeFileID)
		if err != nil {
			e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("load attached networks: %w", err))
			return
		}
		volumes, err := e.attachments.ListVolumesForComposeFile(ctx, organizationID, operation.ComposeFileID)
		if err != nil {
			e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("load attached volumes: %w", err))
			return
		}

		rendered, err := RenderCompose(composeFile.ComposeContent, resolved.TemplateValues, resolved.EnvVars, networks, volumes)
		if err != nil {
			e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("render compose file: %w", err))
			return
		}
		files = append(files, RemoteFile{Path: path.Join(deployPath, "docker-compose.yml"), Content: rendered})

		for _, fv := range resolved.FileVariables {
			content, err := SubstituteTemplateVars(fv.Value, resolved.TemplateValues)
			if err != nil {
				e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("render file variable %q: %w", fv.Key, err))
				return
			}
			files = append(files, RemoteFile{Path: path.Join(deployPath, fv.FileTargetPath), Content: content})
		}
	}

	command := fmt.Sprintf(
		`COMPOSE_CMD=$(docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose") && cd %s && $COMPOSE_CMD -f docker-compose.yml %s 2>&1`,
		shellQuote(deployPath), domain.ComposeSubcommand[operation.OperationType],
	)

	target := ConnectionTarget{Host: machine.Host, Port: machine.SSHPort, User: machine.SSHUser, CredentialType: machine.CredentialType, Secret: secret}
	e.logger.Info("running fleet operation", "operation_id", operationID, "operation_type", string(operation.OperationType), "machine_id", machine.ID)

	exitCode, output, err := e.ssh.RunOperation(ctx, target, files, command, func(line string) {
		// Scrub each line BEFORE publishing it - a real safety fix over
		// the ported Python original, which only scrubbed the final
		// persisted output and let secret values flow unredacted
		// through its own live WebSocket stream.
		if pubErr := e.publisher.PublishLine(ctx, operationID, scrubSecrets(line, secretValues)); pubErr != nil {
			e.logger.Error("failed to publish operation log line", "operation_id", operationID, "error", pubErr)
		}
	})
	if err != nil {
		e.finishWithError(ctx, organizationID, operationID, fmt.Errorf("ssh execution: %w", err))
		return
	}

	scrubbedOutput := scrubSecrets(output, secretValues)
	status := domain.OperationStatusSuccess
	if exitCode != 0 {
		status = domain.OperationStatusFailed
	}
	if markErr := e.operations.MarkFinished(ctx, organizationID, operationID, status, &exitCode, scrubbedOutput); markErr != nil {
		e.logger.Error("failed to persist finished operation", "operation_id", operationID, "error", markErr)
	}
	if pubErr := e.publisher.PublishEnd(ctx, operationID, exitCode); pubErr != nil {
		e.logger.Error("failed to publish operation end", "operation_id", operationID, "error", pubErr)
	}
	e.logger.Info("fleet operation finished", "operation_id", operationID, "status", string(status), "exit_code", exitCode)
}

func (e *DeployExecutor) finishWithError(ctx context.Context, organizationID, operationID string, err error) {
	e.logger.Error("fleet operation failed before execution", "operation_id", operationID, "error", err)
	msg := "internal error: " + err.Error()
	if markErr := e.operations.MarkFinished(ctx, organizationID, operationID, domain.OperationStatusFailed, nil, msg); markErr != nil {
		e.logger.Error("failed to persist failed operation", "operation_id", operationID, "error", markErr)
	}
	if pubErr := e.publisher.PublishEnd(ctx, operationID, -1); pubErr != nil {
		e.logger.Error("failed to publish operation end", "operation_id", operationID, "error", pubErr)
	}
}

// secretValuesFor collects every resolved TemplateValues entry as a
// scrub candidate - resolveComposeVariables doesn't tag which keys came
// from a secret-typed Variable once resolved (both are just strings by
// that point), so scrubSecrets conservatively redacts any of them found
// verbatim in real command output. This over-scrubs relative to the
// ported Python original (which only ever replaced known-secret
// values), but erring toward redacting a plain value that happens to
// also appear in output is a strictly safer failure mode than the
// reverse - a real secret leaking through unredacted.
func secretValuesFor(resolved *ResolvedComposeVariables) []string {
	var values []string
	for _, v := range resolved.TemplateValues {
		if v != "" {
			values = append(values, v)
		}
	}
	return values
}

func scrubSecrets(text string, secretValues []string) string {
	for _, v := range secretValues {
		text = strings.ReplaceAll(text, v, MaskedValue)
	}
	return text
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := slugPattern.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}

// shellQuote wraps a path in single quotes for safe inclusion in the
// remote shell command string - deploy paths are operator-controlled
// (Machine.DeployBasePath + a slugified ComposeFile name), not
// end-user-supplied, but quoting costs nothing and avoids a real class
// of shell-injection bug if that assumption ever changes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
