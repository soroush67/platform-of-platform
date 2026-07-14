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

// SeedBuiltinRoles is called once at Control Plane startup
// (docs/architecture/21-deployment.md §4 step 3) - idempotent via
// ON CONFLICT against the partial unique index on (name) WHERE
// organization_id IS NULL (migrations/0001_init.up.sql), verified
// against a real CockroachDB node to actually no-op on a second run
// rather than erroring or duplicating.
func (r *RoleRepository) SeedBuiltinRoles(ctx context.Context) error {
	for name, permissions := range domain.BuiltinRoles {
		encoded, err := json.Marshal(permissions)
		if err != nil {
			return err
		}
		_, err = r.pool.Exec(ctx,
			`INSERT INTO roles (name, permissions) VALUES ($1, $2)
			 ON CONFLICT (name) WHERE organization_id IS NULL DO NOTHING`,
			name, encoded,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
