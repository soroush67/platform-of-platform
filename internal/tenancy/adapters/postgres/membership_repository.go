package postgres

import (
	"context"

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
		`SELECT EXISTS(SELECT 1 FROM organization_memberships WHERE organization_id = $1 AND user_id = $2)
		    OR EXISTS(SELECT 1 FROM service_accounts WHERE organization_id = $1 AND id = $2)`,
		organizationID, subjectID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
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
		`SELECT id, organization_id, user_id, joined_at FROM organization_memberships WHERE organization_id = $1 ORDER BY joined_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []*domain.OrganizationMembership
	for rows.Next() {
		m := &domain.OrganizationMembership{}
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.JoinedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return memberships, tx.Commit(ctx)
}
