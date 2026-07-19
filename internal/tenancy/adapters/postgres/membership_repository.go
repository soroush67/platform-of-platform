package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/tenancy/domain"
)

type MembershipRepository struct {
	pool *pgxpool.Pool
}

func NewMembershipRepository(pool *pgxpool.Pool) *MembershipRepository {
	return &MembershipRepository{pool: pool}
}

// Create inserts a membership row, scoped the same way
// OrganizationRepository.Create scopes an org: set_config to the
// membership's own organization_id for this transaction only, which
// satisfies organization_memberships_isolation's WITH CHECK
// (migrations/0001_init.up.sql) without needing any broader grant.
func (r *MembershipRepository) Create(ctx context.Context, m *domain.OrganizationMembership) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, m.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO organization_memberships (id, organization_id, user_id, joined_at) VALUES ($1, $2, $3, $4)`,
		m.ID, m.OrganizationID, m.UserID, m.JoinedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// IsMember scopes to organizationID (so RLS allows reading rows for
// exactly that org) then checks whether the subject has a row in it -
// this is the real access-control check: unlike organizations' own RLS
// (self-referential, satisfiable by anyone who knows the id), this
// query can genuinely return false for an authenticated subject who
// isn't actually a member.
//
// A subject can be a User (organization_memberships, the original
// check) OR a ServiceAccount belonging to this org (service_accounts,
// migrations/0017_service_accounts_api_keys.up.sql) - a ServiceAccount
// has no OrganizationMembership row (it's directly scoped to one org by
// its own organization_id column, never invited/added the way a User
// is), so without this second check, every existing service's IsMember
// gate would reject a real, valid, API-key-authenticated ServiceAccount
// principal outright, before RBAC's own permission check ever ran.
//
// The organization_memberships branch also requires blocked_at IS NULL
// - this one query is the choke point nearly every permission check in
// every context already calls first, so a blocked member fails every
// existing gate identically to a real non-member, with zero changes
// needed anywhere else (BlockMemberService's own doc comment). The
// service_accounts branch is deliberately untouched - blocking is a
// human-membership concept only, not extended to ServiceAccounts here.
func (r *MembershipRepository) IsMember(ctx context.Context, organizationID, subjectID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id = $1 AND user_id = $2 AND blocked_at IS NULL)
		    OR EXISTS(SELECT 1 FROM service_accounts WHERE organization_id = $1 AND id = $2)`,
		organizationID, subjectID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}

// MembershipExists is IsMember's blocked-agnostic sibling - see the
// port's own doc comment (ports.go) for why target-validation in
// Block/Unblock/RemoveMember needs this instead.
func (r *MembershipRepository) MembershipExists(ctx context.Context, organizationID, userID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id = $1 AND user_id = $2)`,
		organizationID, userID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}

// SetBlocked backs BlockMemberService/UnblockMemberService - blocked=true
// sets blocked_at to now(), blocked=false clears it back to NULL.
func (r *MembershipRepository) SetBlocked(ctx context.Context, organizationID, userID string, blocked bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	var blockedAt any
	if blocked {
		blockedAt = time.Now().UTC()
	}

	_, err = tx.Exec(ctx,
		`UPDATE organization_memberships SET blocked_at = $1 WHERE organization_id = $2 AND user_id = $3`,
		blockedAt, organizationID, userID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Delete backs RemoveMemberService - a real, permanent removal of this
// User's membership in this org (the operator's own scoped "Delete"
// meaning). RemoveMemberService itself is what cleans up this User's
// RoleBindings first, via RoleBindingCleaner - this method only ever
// touches organization_memberships.
func (r *MembershipRepository) Delete(ctx context.Context, organizationID, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM organization_memberships WHERE organization_id = $1 AND user_id = $2`, organizationID, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListByOrganization backs the member roster (ListMembersService,
// internal/tenancy/application) - same transaction/query shape as
// ProjectRepository.ListByOrganization.
func (r *MembershipRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.OrganizationMembership, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, user_id, joined_at, blocked_at FROM organization_memberships WHERE organization_id = $1 ORDER BY joined_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []*domain.OrganizationMembership
	for rows.Next() {
		m := &domain.OrganizationMembership{}
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.JoinedAt, &m.BlockedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return memberships, tx.Commit(ctx)
}
