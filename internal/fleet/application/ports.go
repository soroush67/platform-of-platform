package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

// Every mutating repository method below takes an explicit actorUserID,
// even where the domain struct itself also carries a CreatedBy (Network/
// Volume/ComposeFile) - Machine has no CreatedBy field at all (neither
// does the ported Python product's own model), so a uniform explicit
// actor parameter across every method is simpler and less error-prone
// than "sometimes read it off the struct, sometimes pass it in." Each
// implementation writes the real audit-outbox event
// (internal/platform/outbox.Write) in the same transaction as its row
// write - see decision #6 in the Fleet plan for why there's no separate
// audit call anywhere in this context.
type MachineRepository interface {
	Create(ctx context.Context, actorUserID string, m *domain.Machine) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Machine, error)
	ListByOrganization(ctx context.Context, organizationID string, includeArchived bool) ([]*domain.Machine, error)
	Update(ctx context.Context, actorUserID string, m *domain.Machine) error
	// Delete performs a real hard delete - callers catch the FK-
	// violation case (a Machine with real Operation history) and archive
	// instead, matching the ported Python product's own delete-or-
	// archive-fallback behavior.
	Delete(ctx context.Context, actorUserID, organizationID, id string) error
	Archive(ctx context.Context, actorUserID, organizationID, id string) error
}

type NetworkRepository interface {
	Create(ctx context.Context, n *domain.Network) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Network, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Network, error)
	Delete(ctx context.Context, actorUserID, organizationID, id string) error
}

type VolumeRepository interface {
	Create(ctx context.Context, v *domain.Volume) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Volume, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Volume, error)
	Delete(ctx context.Context, actorUserID, organizationID, id string) error
}

type ComposeFileRepository interface {
	Create(ctx context.Context, c *domain.ComposeFile) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.ComposeFile, error)
	GetGlobal(ctx context.Context, organizationID string) (*domain.ComposeFile, bool, error)
	ListByOrganization(ctx context.Context, organizationID string) ([]*domain.ComposeFile, error)
	UpdateContent(ctx context.Context, actorUserID, organizationID, id, content string) error
	// Delete returns domain.ErrComposeFileHasHistory on a real FK
	// violation (real Operation rows reference it) - no archive fallback,
	// ComposeFile has no archived concept unlike Machine.
	Delete(ctx context.Context, actorUserID, organizationID, id string) error
}

type AttachmentRepository interface {
	AttachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error
	DetachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error
	ListNetworksForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Network, error)
	AttachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID, containerPath string) error
	DetachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID string) error
	ListVolumesForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]VolumeAttachmentView, error)
	AttachProject(ctx context.Context, actorUserID, organizationID, composeFileID, projectID string) error
	DetachProject(ctx context.Context, actorUserID, organizationID, composeFileID, projectID string) error
	ListProjectsForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]ProjectSummary, error)
	// ListComposeFilesForProject is the reverse of ListProjectsForComposeFile -
	// "which ComposeFiles are linked to this Project," needed by
	// ProjectDetailPage (Tenancy) which otherwise had no way to show a
	// link made from the ComposeFile side.
	ListComposeFilesForProject(ctx context.Context, organizationID, projectID string) ([]*domain.ComposeFile, error)
}

// ProjectSummary is a minimal read-model for "which Projects is this
// ComposeFile linked to" - Fleet doesn't own the Project aggregate
// (Tenancy does) and can't import tenancy/domain across the context
// boundary, so this carries just the fields the UI needs. Filled by a
// same-DB SQL join in the postgres adapter (same reasoning
// ListNetworksForComposeFile's own join into the sibling networks table
// already uses - no cross-context Go call needed for a plain read).
type ProjectSummary struct {
	ID   string
	Name string
	Slug string
}

// ProjectChecker is Fleet's own copy of the identically-shaped port
// Workspace already declares into Tenancy (internal/workspace/
// application/ports.go) - "does this project genuinely belong to this
// org," verified before AttachProject accepts a client-supplied
// project_id, the same "don't trust what the client typed" reasoning.
type ProjectChecker interface {
	ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error)
}

// VolumeAttachmentView joins a Volume's own catalog fields with its
// per-attachment ContainerPath - a read-model, not a domain type,
// because it's shaped for exactly one call site (rendering + the list
// endpoint), same "read-model lives in application, not domain"
// posture as tenancy's own MemberSummary.
type VolumeAttachmentView struct {
	Volume        *domain.Volume
	ContainerPath string
}

type VariableRepository interface {
	Create(ctx context.Context, actorUserID string, v *domain.Variable) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Variable, error)
	ListByComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Variable, error)
	Update(ctx context.Context, actorUserID string, v *domain.Variable) error
	Delete(ctx context.Context, actorUserID, organizationID, id string) error
}

type OperationRepository interface {
	Create(ctx context.Context, o *domain.Operation) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Operation, error)
	ListByOrganization(ctx context.Context, organizationID string, composeFileID, machineID string) ([]*domain.Operation, error)
	// TryClaim is the atomic compare-and-swap DeployExecutor uses to
	// take ownership of a queued Operation - same shape as Execution's
	// own TryStartApplying, needed for exactly the same reason (a
	// redelivered/duplicate poll must not double-execute the same row).
	TryClaim(ctx context.Context, organizationID, id string) (bool, error)
	MarkFinished(ctx context.Context, organizationID, id string, status domain.OperationStatus, exitCode *int, output string) error
}

// OperationCandidate/QueuedOperationScanner - DeployExecutor's own
// cross-org discovery half of the claim (see reap_stale_runs.go's
// FindStaleApplyingRuns for the identical precedent): operations is
// tenant-facing (unlike outbox_events, which deliberately has no RLS
// for exactly this reason), so a cross-org scan needs the root pool,
// not the RLS-scoped app pool OperationRepository above is built on.
type OperationCandidate struct {
	OperationID    string
	OrganizationID string
}

type QueuedOperationScanner interface {
	FindQueuedCandidates(ctx context.Context, limit int) ([]OperationCandidate, error)
}

// MembershipChecker/PermissionChecker - this context's own copies of
// the same port shape every other context declares locally
// (docs/architecture/18-backend-structure.md §3's dependency-inversion
// rule).
type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}

// SecretMountChecker/SecretResolver - Fleet never imports secrets/domain
// (this codebase's no-cross-context-import rule), same shape as
// Variables' own identically-named ports into the Secrets context. A
// Machine's SSH credential and a secret-typed fleet_variables row both
// go through these.
type SecretMountChecker interface {
	SecretMountExists(ctx context.Context, organizationID, mountID string) (bool, error)
}

// SecretMountCheckerFunc lets main.go adapt a method value straight into
// this port, matching Variables' own identical precedent.
type SecretMountCheckerFunc func(ctx context.Context, organizationID, mountID string) (bool, error)

func (f SecretMountCheckerFunc) SecretMountExists(ctx context.Context, organizationID, mountID string) (bool, error) {
	return f(ctx, organizationID, mountID)
}

// SecretResolver is wired in main.go to secrets/application's own
// ResolveSecretService.ResolveValue with zero adapter glue, same
// structural-satisfaction pattern Variables' own SecretResolver uses.
type SecretResolver interface {
	ResolveValue(ctx context.Context, organizationID, mountID, path string) (string, error)
}

// LogPublisher is DeployExecutor's own port into the Redis Pub/Sub
// adapter (adapters/redisstream) - kept as a narrow port (not a direct
// *redis.Client dependency) so the executor's own tests can fake it.
type LogPublisher interface {
	PublishLine(ctx context.Context, operationID, line string) error
	PublishEnd(ctx context.Context, operationID string, exitCode int) error
}

// ConnectionTarget/RemoteFile/SSHRunner - Fleet's own port into the real
// SSH/SFTP adapter (adapters/ssh) - kept narrow so DeployExecutor/
// TestMachineConnectionService/CheckMachineConnectionService can all
// share one fake in tests without pulling in a real network dependency.
type ConnectionTarget struct {
	Host           string
	Port           int
	User           string
	CredentialType domain.CredentialType
	Secret         string // the already-resolved plaintext key/password
}

type RemoteFile struct {
	Path    string // absolute path on the target host
	Content string
}

type SSHRunner interface {
	Probe(ctx context.Context, target ConnectionTarget) (domain.ConnectionStatus, domain.DockerStatus, error)
	RunOperation(ctx context.Context, target ConnectionTarget, files []RemoteFile, command string, onLine func(string)) (exitCode int, output string, err error)
}
