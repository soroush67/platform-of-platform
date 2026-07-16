package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/tenancy/domain"
)

type TeamRepository struct {
	pool *pgxpool.Pool
}

func NewTeamRepository(pool *pgxpool.Pool) *TeamRepository {
	return &TeamRepository{pool: pool}
}

func (r *TeamRepository) Create(ctx context.Context, team *domain.Team) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, team.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO teams (id, organization_id, name, created_at) VALUES ($1, $2, $3, $4)`,
		team.ID, team.OrganizationID, team.Name, team.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Exists is what CreateRoleBindingService uses to validate a
// subject_type='team' binding actually points at a real Team in this
// Organization - same "validate the subject before creating a grant for
// it" posture as the membership checks every other write path in this
// codebase already does.
func (r *TeamRepository) TeamExists(ctx context.Context, organizationID, teamID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teams WHERE id = $1 AND organization_id = $2)`, teamID, organizationID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, tx.Commit(ctx)
}

func (r *TeamRepository) AddMember(ctx context.Context, membership *domain.TeamMembership) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, membership.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO team_memberships (id, team_id, organization_id, user_id, joined_at) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (team_id, user_id) DO NOTHING`,
		membership.ID, membership.TeamID, membership.OrganizationID, membership.UserID, membership.JoinedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *TeamRepository) RemoveMember(ctx context.Context, organizationID, teamID, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM team_memberships WHERE team_id = $1 AND user_id = $2`, teamID, userID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *TeamRepository) GetByID(ctx context.Context, organizationID, teamID string) (*domain.Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var team domain.Team
	err = tx.QueryRow(ctx,
		`SELECT id, organization_id, name, created_at FROM teams WHERE id = $1 AND organization_id = $2`,
		teamID, organizationID,
	).Scan(&team.ID, &team.OrganizationID, &team.Name, &team.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrTeamNotFound
		}
		return nil, err
	}

	return &team, tx.Commit(ctx)
}

func (r *TeamRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*domain.Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, organization_id, name, created_at FROM teams WHERE organization_id = $1 ORDER BY created_at`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*domain.Team
	for rows.Next() {
		var team domain.Team
		if err := rows.Scan(&team.ID, &team.OrganizationID, &team.Name, &team.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, &team)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return teams, tx.Commit(ctx)
}
