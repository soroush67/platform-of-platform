package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrTeamAlreadyExists
		}
		return err
	}

	return tx.Commit(ctx)
}

// Update renames a Team in place - UpdateTeamService's own reason this
// exists: fixing a typo shouldn't mean delete-and-recreate (which would
// also silently drop every existing membership and RoleBinding pointing
// at the old id). Same 23505->ErrTeamAlreadyExists mapping Create
// already has - renaming to a name already taken in this org is a real,
// clean 409, not a 500.
func (r *TeamRepository) Update(ctx context.Context, team *domain.Team) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, team.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE teams SET name = $1 WHERE id = $2 AND organization_id = $3`, team.Name, team.ID, team.OrganizationID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrTeamAlreadyExists
		}
		return err
	}

	return tx.Commit(ctx)
}

// Delete removes a Team and its own team_memberships in one transaction -
// team_memberships.team_id has no ON DELETE CASCADE
// (migrations/0012_teams_and_org_archival.up.sql), so deleting the teams
// row first would hit a real FK violation, not just leave an orphan.
// RoleBindings granted TO this team are a separate, cross-context
// concern - DeleteTeamService cleans those up first, via
// RoleBindingCleaner, before ever calling this.
func (r *TeamRepository) Delete(ctx context.Context, organizationID, teamID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM team_memberships WHERE team_id = $1 AND organization_id = $2`, teamID, organizationID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM teams WHERE id = $1 AND organization_id = $2`, teamID, organizationID); err != nil {
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

// ListMembers backs the new `GET /orgs/{org}/teams/{team}/members` roster
// endpoint - same shape as MembershipRepository.ListByOrganization
// (organization_repository.go's own sibling), ordered by joined_at like
// every other roster in this codebase.
func (r *TeamRepository) ListMembers(ctx context.Context, organizationID, teamID string) ([]*domain.TeamMembership, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, team_id, organization_id, user_id, joined_at FROM team_memberships WHERE team_id = $1 AND organization_id = $2 ORDER BY joined_at`,
		teamID, organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []*domain.TeamMembership
	for rows.Next() {
		var m domain.TeamMembership
		if err := rows.Scan(&m.ID, &m.TeamID, &m.OrganizationID, &m.UserID, &m.JoinedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return memberships, tx.Commit(ctx)
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
