package application

import (
	"context"

	"platform-of-platform/internal/execution/domain"
)

type RunRepository interface {
	Create(ctx context.Context, run *domain.Run) error
	GetByID(ctx context.Context, organizationID, id string) (*domain.Run, error)
	ListByWorkspace(ctx context.Context, organizationID, workspaceID string) ([]*domain.Run, error)
	// Update persists a Run whose in-memory state was already mutated by
	// a domain method (Run.Cancel()) - same "domain decides, repository
	// just persists" split as every other write in this codebase.
	// actorUserID is attached to the outbox event the adapter writes in
	// the same transaction, same reasoning as OrganizationRepository.Create.
	Update(ctx context.Context, run *domain.Run, actorUserID string) error
}

// WorkspaceLocker is this context's port into Workspace for the one
// property Stage 3 §6 calls out as load-bearing: "only one Run may be
// in a non-terminal status per Workspace at a time - this IS the
// Workspace's lock_status." TryLock returns (false, nil), not an error,
// when the workspace was already locked - same (bool, error) shape as
// every other cross-context check in this codebase.
type WorkspaceLocker interface {
	TryLock(ctx context.Context, organizationID, workspaceID, runID string) (bool, error)
	Unlock(ctx context.Context, organizationID, workspaceID, runID string) error
}

// WorkspaceChecker, MembershipChecker, PermissionChecker - same shape
// and reasoning as every other context's own-declared ports into
// Tenancy/RBAC/Workspace (docs/architecture/18-backend-structure.md §3).
type WorkspaceChecker interface {
	WorkspaceExists(ctx context.Context, organizationID, projectID, workspaceID string) (bool, error)
}

type MembershipChecker interface {
	IsMember(ctx context.Context, organizationID, userID string) (bool, error)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error)
}
