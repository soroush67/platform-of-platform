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
	ErrVariableNotFound        = errors.New("variable not found")
	ErrOperationNotFound       = errors.New("operation not found")
	ErrOperationNotClaimed     = errors.New("operation was not in a claimable state")
	ErrForbidden               = errors.New("forbidden")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
