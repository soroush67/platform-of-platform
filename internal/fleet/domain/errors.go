// Package domain holds the Fleet context's pure Go types - Machines,
// Networks, Volumes, ComposeFiles, per-ComposeFile Variables, and deploy
// Operations, ported from a separate Python/FastAPI product
// (/home/soroush/compose-platform) that centrally manages/deploys
// docker-compose files across multiple remote machines over SSH.
// Deliberately Phase 1 only: Groups, a ChangeRequest approval workflow,
// GitLab ingestion, and NotificationSettings are all built there but not
// ported here yet - see this context's own package-level plan notes.
package domain

import "errors"

var (
	ErrMachineNotFound         = errors.New("machine not found")
	ErrMachineHasHistory       = errors.New("machine has operation history and cannot be hard-deleted")
	ErrNetworkNotFound         = errors.New("network not found")
	ErrNetworkInUse            = errors.New("network is still attached to a compose file")
	ErrVolumeNotFound          = errors.New("volume not found")
	ErrVolumeInUse             = errors.New("volume is still attached to a compose file")
	ErrComposeFileNotFound     = errors.New("compose file not found")
	ErrGlobalComposeFileExists = errors.New("this organization already has a global compose file")
	// ErrComposeFileHasHistory mirrors ErrMachineHasHistory - operations
	// has no ON DELETE CASCADE on compose_file_id and no DELETE grant at
	// all (deliberately immutable history), so a ComposeFile with real
	// deploy history genuinely can't be hard-deleted. Unlike Machine,
	// ComposeFile has no archive concept to fall back to - this
	// propagates straight to the caller as a real 409.
	ErrComposeFileHasHistory = errors.New("compose file has operation history and cannot be deleted")
	ErrVariableNotFound      = errors.New("variable not found")
	ErrOperationNotFound     = errors.New("operation not found")
	ErrOperationNotClaimed   = errors.New("operation was not in a claimable state")
	ErrForbidden             = errors.New("forbidden")
	// ErrProjectNotFound - Fleet doesn't own Project (Tenancy does), but
	// AttachProject needs its own sentinel for "the project_id the client
	// supplied doesn't exist in this org," verified via ProjectChecker,
	// same "don't trust what the client typed" reasoning as every other
	// cross-context reference in this codebase.
	ErrProjectNotFound = errors.New("project not found")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
