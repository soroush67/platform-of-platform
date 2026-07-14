// Package postgres - see the Tenancy adapter's identically-named package
// for why "postgres" names the wire protocol, not the actual engine
// (CockroachDB, docs/architecture/05-database.md §0).
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
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
		`INSERT INTO users (id, username, email, auth_source, external_id, status, mfa_enrolled, created_at, password_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID, user.Username, user.Email, string(user.AuthSource), user.ExternalID, user.Status, user.MFAEnrolled, user.CreatedAt, user.PasswordHash,
	)
	return err
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, auth_source, external_id, status, mfa_enrolled, created_at, password_hash
		 FROM users WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.AuthSource, &user.ExternalID, &user.Status, &user.MFAEnrolled, &user.CreatedAt, &user.PasswordHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetByID is what RefreshTokenService/PasswordResetService use - both
// only ever have a user_id at hand (from a validated token row, not a
// login form), unlike AuthenticateService's own username-keyed lookup.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, auth_source, external_id, status, mfa_enrolled, created_at, password_hash
		 FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.AuthSource, &user.ExternalID, &user.Status, &user.MFAEnrolled, &user.CreatedAt, &user.PasswordHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetByEmail is PasswordResetService's own lookup for the "request a
// reset" step - a real deployment's reset form asks for an email
// address, not a username (the recipient of the reset link, after all,
// proves control of the email, not the username).
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, email, auth_source, external_id, status, mfa_enrolled, created_at, password_hash
		 FROM users WHERE email = $1`,
		email,
	).Scan(&user.ID, &user.Username, &user.Email, &user.AuthSource, &user.ExternalID, &user.Status, &user.MFAEnrolled, &user.CreatedAt, &user.PasswordHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// UpdatePasswordHash is PasswordResetService's own write - the only
// place in this codebase that changes an existing User's credential
// after creation.
func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash)
	return err
}
