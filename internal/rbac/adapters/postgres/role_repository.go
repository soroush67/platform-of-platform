package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/rbac/domain"
)

type RoleRepository struct {
	pool *pgxpool.Pool
}

func NewRoleRepository(pool *pgxpool.Pool) *RoleRepository {
	return &RoleRepository{pool: pool}
}

// SeedBuiltinRoles runs at every Control Plane startup
// (docs/architecture/21-deployment.md §4 step 3) - an UPSERT against the
// partial unique index on (name) WHERE organization_id IS NULL
// (migrations/0001_init.up.sql), not a plain "insert if missing." A
// built-in role's permission set is code, not user data (Stage 13 §4:
// custom Roles compose existing Permissions, they don't redefine
// built-in ones) - when domain.BuiltinRoles changes (as it did adding
// workspace:read/workspace:manage), every existing deployment's roles
// table needs to pick that up on its next restart, not stay frozen at
// whatever was seeded the first time it ever started. Verified against
// a real CockroachDB node that DO UPDATE against a partial-index
// conflict target actually overwrites permissions on a second run,
// not just no-ops like DO NOTHING would.
func (r *RoleRepository) SeedBuiltinRoles(ctx context.Context) error {
	for name, permissions := range domain.BuiltinRoles {
		encoded, err := json.Marshal(permissions)
		if err != nil {
			return err
		}
		_, err = r.pool.Exec(ctx,
			`INSERT INTO roles (name, permissions) VALUES ($1, $2)
			 ON CONFLICT (name) WHERE organization_id IS NULL
			 DO UPDATE SET permissions = excluded.permissions`,
			name, encoded,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// Create inserts a custom, organization-scoped Role
// (docs/architecture/13-module-identity-rbac-tenancy.md §3's
// `POST /orgs/{org}/roles`). roles_org_name_unique
// (migrations/0001_init.up.sql) is what actually enforces "unique name
// per org" - a duplicate name surfaces here as a real Postgres unique
// violation, which CreateRoleService maps to domain.ErrRoleAlreadyExists.
func (r *RoleRepository) Create(ctx context.Context, role *domain.Role) error {
	if role.OrganizationID == nil {
		return &domain.ValidationError{Message: "custom roles must have an organization_id"}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, *role.OrganizationID); err != nil {
		return err
	}

	encoded, err := json.Marshal(role.Permissions)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, organization_id, name, permissions) VALUES ($1, $2, $3, $4)`,
		role.ID, *role.OrganizationID, role.Name, encoded,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrRoleAlreadyExists
		}
		return err
	}

	return tx.Commit(ctx)
}

// ListForOrganization returns every Role visible to this org - built-in
// (organization_id IS NULL, visible everywhere per roles_isolation's own
// RLS policy) plus this org's own custom ones - matching
// docs/architecture/13-module-identity-rbac-tenancy.md §3's
// "GET .../roles lists built-in + org-custom roles."
func (r *RoleRepository) ListForOrganization(ctx context.Context, organizationID string) ([]*domain.Role, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `SELECT id, organization_id, name, permissions FROM roles ORDER BY organization_id NULLS FIRST, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*domain.Role
	for rows.Next() {
		var role domain.Role
		var permissionsRaw []byte
		if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Name, &permissionsRaw); err != nil {
			return nil, err
		}
		var permissionNames []string
		if err := json.Unmarshal(permissionsRaw, &permissionNames); err != nil {
			return nil, err
		}
		role.Permissions = make([]domain.Permission, len(permissionNames))
		for i, p := range permissionNames {
			role.Permissions[i] = domain.Permission(p)
		}
		roles = append(roles, &role)
	}

	return roles, rows.Err()
}

// GetByID is used by CreateRoleBindingService to validate a role_id
// exists and belongs to this org (or is a built-in, organization_id
// NULL) before binding it - docs/architecture/03-domain-model.md §4's
// invariant: "a RoleBinding's scope must be a resource within the same
// Organization as the Role it references."
func (r *RoleRepository) GetByID(ctx context.Context, organizationID, roleID string) (*domain.Role, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var role domain.Role
	var permissionsRaw []byte
	err = tx.QueryRow(ctx, `SELECT id, organization_id, name, permissions FROM roles WHERE id = $1`, roleID).
		Scan(&role.ID, &role.OrganizationID, &role.Name, &permissionsRaw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrRoleNotFound
		}
		return nil, err
	}
	var permissionNames []string
	if err := json.Unmarshal(permissionsRaw, &permissionNames); err != nil {
		return nil, err
	}
	role.Permissions = make([]domain.Permission, len(permissionNames))
	for i, p := range permissionNames {
		role.Permissions[i] = domain.Permission(p)
	}

	return &role, tx.Commit(ctx)
}
