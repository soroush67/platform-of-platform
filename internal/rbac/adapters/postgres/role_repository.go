package postgres

import (
	"context"
	"encoding/json"

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
