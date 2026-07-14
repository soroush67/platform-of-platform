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
// exactly that org) then checks whether userID has a row in it - this
// is the real access-control check: unlike organizations' own RLS
// (self-referential, satisfiable by anyone who knows the id), this
// query can genuinely return false for an authenticated user who isn't
// actually a member.
func (r *MembershipRepository) IsMember(ctx context.Context, organizationID, userID string) (bool, error) {
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
