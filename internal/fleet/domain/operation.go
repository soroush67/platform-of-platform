package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OperationType mirrors the ported Python product's own 9 operation
// types exactly.
type OperationType string

const (
	OperationTypeDeploy  OperationType = "deploy"
	OperationTypeUp      OperationType = "up"
	OperationTypeDown    OperationType = "down"
	OperationTypeRestart OperationType = "restart"
	OperationTypePull    OperationType = "pull"
	OperationTypeBuild   OperationType = "build"
	OperationTypeStop    OperationType = "stop"
	OperationTypeStart   OperationType = "start"
	OperationTypeRemove  OperationType = "remove"
)

func (t OperationType) Valid() bool {
	_, ok := ComposeSubcommand[t]
	return ok
}

// ComposeSubcommand maps each OperationType to the real `docker compose`
// subcommand DeployExecutor runs over SSH - only OperationTypeDeploy
// also (re)renders and writes the compose file first; every other type
// runs its subcommand against whatever is already on disk at the
// Machine's deploy path, exactly like the Python original.
var ComposeSubcommand = map[OperationType]string{
	OperationTypeDeploy:  "up -d",
	OperationTypeUp:      "up -d",
	OperationTypeDown:    "down",
	OperationTypeRestart: "restart",
	OperationTypePull:    "pull",
	OperationTypeBuild:   "build",
	OperationTypeStop:    "stop",
	OperationTypeStart:   "start",
	OperationTypeRemove:  "rm -f -s",
}

// OperationStatus adds a real "queued" state the Python original never
// needed - its Celery task picks up a just-inserted row near-instantly
// (`run_operation.delay(...)` called right after the row commits with
// status=running already set). Go's own DeployExecutor
// (application/deploy_executor.go) is a ticker-poll Runnable, not an
// immediate dispatch - there's a real window between TriggerOperation's
// INSERT and the next poll tick where the row is genuinely queued, not
// running. Modeled honestly rather than lying about started_at.
type OperationStatus string

const (
	OperationStatusQueued  OperationStatus = "queued"
	OperationStatusRunning OperationStatus = "running"
	OperationStatusSuccess OperationStatus = "success"
	OperationStatusFailed  OperationStatus = "failed"
)

// Operation is one real deploy/up/down/etc attempt against one Machine.
// Output is the full (secret-scrubbed) stdout+stderr, persisted once the
// run finishes - never partial, matching the Python original's own
// "only the final Operation.output write is durable, the live stream is
// ephemeral" posture (DeployExecutor still publishes each line to Redis
// as it runs, for the SSE endpoint - that's transport, not storage).
type Operation struct {
	ID             string
	OrganizationID string
	ComposeFileID  string
	MachineID      string
	OperationType  OperationType
	Status         OperationStatus
	TriggeredBy    string
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	ExitCode       *int
	Output         string
}

func NewOperation(organizationID, composeFileID, machineID string, opType OperationType, triggeredBy string) (*Operation, error) {
	if organizationID == "" {
		return nil, &ValidationError{Message: "organization_id is required"}
	}
	if composeFileID == "" {
		return nil, &ValidationError{Message: "compose_file_id is required"}
	}
	if machineID == "" {
		return nil, &ValidationError{Message: "machine_id is required"}
	}
	if !opType.Valid() {
		return nil, &ValidationError{Message: fmt.Sprintf("operation_type %q is not a recognized operation type", opType)}
	}
	if triggeredBy == "" {
		return nil, &ValidationError{Message: "triggered_by is required"}
	}

	return &Operation{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		ComposeFileID:  composeFileID,
		MachineID:      machineID,
		OperationType:  opType,
		Status:         OperationStatusQueued,
		TriggeredBy:    triggeredBy,
		CreatedAt:      time.Now().UTC(),
	}, nil
}
