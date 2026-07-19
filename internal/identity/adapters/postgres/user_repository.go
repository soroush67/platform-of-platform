// Package postgres - see the Tenancy adapter's identically-named package
// for why "postgres" names the wire protocol, not the actual engine
// (CockroachDB, docs/architecture/05-database.md §0).
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/identity/domain"
)

// userColumns is shared by every SELECT below - Create/GetByUsername/
// GetByID/GetByEmail all scan the exact same 10-column shape into a
// *domain.User, previously repeated verbatim at each call site.
const userColumns = `id, username, email, auth_source, external_id, status, mfa_enrolled, created_at, password_hash, is_platform_admin`

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func scanUser(row interface {
	Scan(dest ...any) error
}, user *domain.User) error {
	return row.Scan(&user.ID, &user.Username, &user.Email, &user.AuthSource, &user.ExternalID, &user.Status, &user.MFAEnrolled, &user.CreatedAt, &user.PasswordHash, &user.IsPlatformAdmin)
}

// Create inserts a new User row. No RLS, no app.current_org_id scoping -
// the users table deliberately carries no organization_id column
// (migrations/0001_init.up.sql), matching the domain model's "User is
// platform-global" invariant, so a plain pool.Exec on any pooled
// connection is correct here, unlike the Tenancy adapter's Create.
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, username, email, auth_source, external_id, status, mfa_enrolled, created_at, password_hash, is_platform_admin)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		user.ID, user.Username, user.Email, string(user.AuthSource), user.ExternalID, user.Status, user.MFAEnrolled, user.CreatedAt, user.PasswordHash, user.IsPlatformAdmin,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE username = $1`, username), &user)
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
	err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id), &user)
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
	err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email), &user)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// Count backs the first-user bootstrap check
// (httpserver.RequireAuthOrFirstUserBootstrap) - a fresh deployment with
// zero rows in this table is the one case POST /users is allowed to run
// unauthenticated, so an operator can create their very first login-
// capable account at all. Every other caller (count > 0) goes through
// the normal RequireAuth path.
func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count)
	return count, err
}

// GetUser is a thin sibling of GetByID, returned as primitives rather
// than *domain.User - it's what satisfies Tenancy's own UserReader port
// (internal/tenancy/application/ports.go) for the member roster
// (ListMembersService), which can't accept an identity/domain.User
// without importing this context's domain package (the same
// dependency-inversion reasoning RoleAssigner's own doc comment gives).
// found=false, not an error, on no row - a roster resolving one member's
// user record that's since been deleted shouldn't fail the whole list.
func (r *UserRepository) GetUser(ctx context.Context, id string) (username, email string, found bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT username, email FROM users WHERE id = $1`, id).Scan(&username, &email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return username, email, true, nil
}

// ListAll is UserReader.ListAll's structural satisfaction (Tenancy's
// own port, internal/tenancy/application/ports.go) - backs the Members
// page's "add existing user" picker (ListAvailableUsersService), since
// User creation is platform-global and has no reverse index from "every
// User" back to "which orgs is this User NOT in yet."
func (r *UserRepository) ListAll(ctx context.Context) ([]struct{ ID, Username, Email string }, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, username, email FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []struct{ ID, Username, Email string }
	for rows.Next() {
		var u struct{ ID, Username, Email string }
		if err := rows.Scan(&u.ID, &u.Username, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdatePasswordHash is PasswordResetService's own write - the only
// place in this codebase that changes an existing User's credential
// after creation.
func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash)
	return err
}

// IsPlatformAdmin/SetPlatformAdmin back Tenancy's own PlatformAdminChecker/
// PlatformAdminSetter ports (internal/tenancy/application/ports.go) -
// this repository satisfies both structurally, same "one adapter type
// satisfies many contexts' ports" convention roleBindingRepo already
// demonstrates in main.go.
func (r *UserRepository) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	var isAdmin bool
	err := r.pool.QueryRow(ctx, `SELECT is_platform_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, domain.ErrUserNotFound
		}
		return false, err
	}
	return isAdmin, nil
}

func (r *UserRepository) SetPlatformAdmin(ctx context.Context, userID string, isAdmin bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET is_platform_admin = $2 WHERE id = $1`, userID, isAdmin)
	return err
}
