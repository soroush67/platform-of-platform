package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/rbac/domain"
)

type RoleBindingRepository struct {
	pool *pgxpool.Pool
}

func NewRoleBindingRepository(pool *pgxpool.Pool) *RoleBindingRepository {
	return &RoleBindingRepository{pool: pool}
}

// AssignRole binds a built-in role (by name) to a user at organization
// scope. Same set_config(...)-scoped-transaction pattern as every other
// org-scoped write in this codebase (docs/architecture/05-database.md
// §1) - scopes to the organization the binding belongs to, which also
// happens to be exactly the scope needed to look the built-in role's id
// up: built-in roles (organization_id IS NULL) are visible under any
// scope per roles_isolation's policy (migrations/0001_init.up.sql), so
// no separate unscoped lookup is needed.
func (r *RoleBindingRepository) AssignRole(ctx context.Context, organizationID, userID, roleName string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	var roleID string
	err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1 AND organization_id IS NULL`, roleName).Scan(&roleID)
	if err != nil {
		return err
	}

	binding := domain.NewOrganizationOwnerBinding(organizationID, roleID, userID)
	_, err = tx.Exec(ctx,
		`INSERT INTO role_bindings (id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		binding.ID, binding.OrganizationID, binding.RoleID, binding.SubjectType, binding.SubjectID, binding.ScopeType, binding.ScopeID, binding.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// HasPermission is the actual authorization check every gated action
// calls through (docs/architecture/03-domain-model.md §4: RBAC answers
// "can this subject touch this resource class at all"). The
// `r.permissions @> to_jsonb($3::text)` containment check was verified
// against a real CockroachDB node before writing this, not assumed from
// Postgres jsonb familiarity.
func (r *RoleBindingRepository) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM role_bindings rb
			JOIN roles r ON r.id = rb.role_id
			WHERE rb.organization_id = $1
			  AND rb.subject_type = 'user' AND rb.subject_id = $2
			  AND rb.scope_type = 'organization' AND rb.scope_id = $1
			  AND r.permissions @> to_jsonb($3::text)
		)`,
		organizationID, userID, permission,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}
