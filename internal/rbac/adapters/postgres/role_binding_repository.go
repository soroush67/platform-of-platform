package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
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

	binding := domain.NewOrganizationScopeBinding(organizationID, roleID, userID)
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

// ReplaceRole is real "change this member's role" semantics - AssignRole
// alone is additive (a second call binds a *second* role, and since
// HasPermission is an OR across every matching binding, the old role's
// permissions would keep applying too, not actually change anything).
// This deletes any existing organization-scope binding for the user
// first, then inserts the new one, atomically in one transaction - a
// member genuinely has exactly one role at organization scope after
// this call, not a growing accumulation of every role they were ever
// assigned.
func (r *RoleBindingRepository) ReplaceRole(ctx context.Context, organizationID, userID, roleName string) error {
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

	_, err = tx.Exec(ctx,
		`DELETE FROM role_bindings WHERE organization_id = $1 AND subject_type = 'user' AND subject_id = $2 AND scope_type = 'organization' AND scope_id = $1`,
		organizationID, userID,
	)
	if err != nil {
		return err
	}

	binding := domain.NewOrganizationScopeBinding(organizationID, roleID, userID)
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

// HasPermission is the actual authorization check most gated actions call
// through (docs/architecture/03-domain-model.md §4: RBAC answers "can
// this subject touch this resource class at all"). A thin wrapper over
// HasPermissionAtScope with no project/workspace scope - the right
// choice for actions that have nothing beneath organization to bind
// against (create a Project, add a member, read the audit log) per that
// same doc's own Invariant. Every *existing* caller's interface
// declaration still matches this exact 4-arg signature unchanged - this
// method's behavior only grew (team-mediated bindings now count too),
// nothing about its contract narrowed.
func (r *RoleBindingRepository) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	return r.HasPermissionAtScope(ctx, organizationID, userID, permission, nil, nil)
}

// HasPermissionAtScope is the scope-aware evaluation
// docs/architecture/03-domain-model.md §4 actually specifies: "a binding
// at a higher scope (Organization) implies the grant at every resource
// beneath it (Projects, Workspaces)." projectID/workspaceID are nil when
// the action being checked has no such resource yet (e.g. creating a
// Workspace has a projectID but no workspaceID; creating a Project has
// neither). Deliberately pure-additive OR across every matching binding
// (Kubernetes-RBAC-style), NOT the doc's own "...unless a more specific
// binding narrows it" - narrowing would need an explicit-deny concept
// this RoleBinding model doesn't have (every binding today is a grant,
// never a deny); a real, named simplification, not silently dropped.
// Matches bindings for the user directly AND for any team the user
// belongs to (`team_memberships`), the mechanism that makes Team-subject
// bindings (docs/architecture/03-domain-model.md §2) actually work.
func (r *RoleBindingRepository) HasPermissionAtScope(ctx context.Context, organizationID, userID, permission string, projectID, workspaceID *string) (bool, error) {
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
			  AND (
			    (rb.subject_type = 'user' AND rb.subject_id = $2)
			    OR (rb.subject_type = 'team' AND rb.subject_id IN (
			          SELECT team_id FROM team_memberships WHERE user_id = $2 AND organization_id = $1
			    ))
			  )
			  AND (
			    (rb.scope_type = 'organization' AND rb.scope_id = $1)
			    OR ($3::uuid IS NOT NULL AND rb.scope_type = 'project' AND rb.scope_id = $3)
			    OR ($4::uuid IS NOT NULL AND rb.scope_type = 'workspace' AND rb.scope_id = $4)
			  )
			  AND r.permissions @> to_jsonb($5::text)
		)`,
		organizationID, userID, projectID, workspaceID, permission,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, tx.Commit(ctx)
}

// Create inserts an arbitrary RoleBinding - the real
// `POST /orgs/{org}/role-bindings` endpoint
// (docs/architecture/13-module-identity-rbac-tenancy.md §3), unlike
// AssignRole/ReplaceRole above which are hardcoded to the built-in-role,
// user-subject, organization-scope bootstrap path. CreateRoleBindingService
// is what validates the role/subject/scope actually exist and belong to
// this org before calling this - this method trusts its caller the same
// way every other repository in this codebase does.
func (r *RoleBindingRepository) Create(ctx context.Context, binding *domain.RoleBinding) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, binding.OrganizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO role_bindings (id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (role_id, subject_type, subject_id, scope_type, scope_id) DO NOTHING`,
		binding.ID, binding.OrganizationID, binding.RoleID, binding.SubjectType, binding.SubjectID, binding.ScopeType, binding.ScopeID, binding.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListForSubject implements
// `GET /orgs/{org}/role-bindings?subject_id=...` - "what can this
// subject do, and where" (docs/architecture/13-module-identity-rbac-
// tenancy.md §3). Empty subjectID lists every binding in the org
// (an admin's "show me every grant" view).
func (r *RoleBindingRepository) ListForSubject(ctx context.Context, organizationID, subjectID string) ([]*domain.RoleBinding, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return nil, err
	}

	var rows pgx.Rows
	if subjectID == "" {
		rows, err = tx.Query(ctx, `SELECT id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, created_at FROM role_bindings WHERE organization_id = $1 ORDER BY created_at`, organizationID)
	} else {
		rows, err = tx.Query(ctx, `SELECT id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, created_at FROM role_bindings WHERE organization_id = $1 AND subject_id = $2 ORDER BY created_at`, organizationID, subjectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []*domain.RoleBinding
	for rows.Next() {
		var b domain.RoleBinding
		if err := rows.Scan(&b.ID, &b.OrganizationID, &b.RoleID, &b.SubjectType, &b.SubjectID, &b.ScopeType, &b.ScopeID, &b.CreatedAt); err != nil {
			return nil, err
		}
		bindings = append(bindings, &b)
	}

	return bindings, rows.Err()
}
