// Package postgres - see the Tenancy adapter's identically-named package
// for why "postgres" names the wire protocol, not the actual engine
// (CockroachDB, docs/architecture/05-database.md §0).
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/identity/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a new User row. No RLS, no app.current_org_id scoping -
// the users table deliberately carries no organization_id column
// (migrations/0001_init.up.sql), matching the domain model's "User is
// platform-global" invariant, so a plain pool.Exec on any pooled
// connection is correct here, unlike the Tenancy adapter's Create.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, username, email, auth_source, external_id, status, mfa_enrolled, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		user.ID, user.Username, user.Email, string(user.AuthSource), user.ExternalID, user.Status, user.MFAEnrolled, user.CreatedAt,
	)
	return err
}
