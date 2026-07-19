package postgres

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/platform/principal"
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
		`INSERT INTO role_bindings (id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, effect, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		binding.ID, binding.OrganizationID, binding.RoleID, binding.SubjectType, binding.SubjectID, binding.ScopeType, binding.ScopeID, binding.Effect, binding.CreatedAt,
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
		`INSERT INTO role_bindings (id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, effect, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		binding.ID, binding.OrganizationID, binding.RoleID, binding.SubjectType, binding.SubjectID, binding.ScopeType, binding.ScopeID, binding.Effect, binding.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetOrgScopeRoleName satisfies Tenancy's own RoleReader port
// (internal/tenancy/application/ports.go) for the member roster
// (ListMembersService) - same WHERE clause ReplaceRole's own DELETE
// above uses to find "the" org-scope binding for a member, joined to
// roles for its name. Not restricted to built-in roles only (unlike
// AssignRole/ReplaceRole, which only ever assign one) - a member's
// org-scope binding could in principle point at a custom role instead,
// created via the general Role Bindings page rather than
// ChangeMemberRoleService, and the roster should show that real name
// too. found=false, not an error, when a member has no org-scope
// binding at all - genuinely possible for a member added outside
// AddMemberService's own AssignRole call.
func (r *RoleBindingRepository) GetOrgScopeRoleName(ctx context.Context, organizationID, userID string) (string, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return "", false, err
	}

	var roleName string
	err = tx.QueryRow(ctx,
		`SELECT roles.name FROM role_bindings
		 JOIN roles ON roles.id = role_bindings.role_id
		 WHERE role_bindings.organization_id = $1 AND role_bindings.subject_type = 'user'
		   AND role_bindings.subject_id = $2 AND role_bindings.scope_type = 'organization' AND role_bindings.scope_id = $1`,
		organizationID, userID,
	).Scan(&roleName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	return roleName, true, tx.Commit(ctx)
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
// beneath it (Projects, Workspaces)... unless a more specific binding
// narrows it." projectID/workspaceID are nil when the action being
// checked has no such resource yet (e.g. creating a Workspace has a
// projectID but no workspaceID; creating a Project has neither).
//
// AWS-IAM-style evaluation, not Kubernetes RBAC's pure-additive-only
// model (the earlier version of this method): every matching binding at
// any scope level is collected first, then any Deny among them wins
// unconditionally over every Allow, regardless of which scope each came
// from - this is the actual mechanism that makes "narrowing" real: an
// Allow at Organization scope plus a Deny at Workspace scope for the
// same permission genuinely denies it at that Workspace, which a
// pure-additive model structurally cannot express (there's no way for
// a "grant" to ever subtract from another grant).
//
// Matches bindings for the subject directly (user OR service_account -
// both are plain subject_id equality checks, no join needed to tell
// them apart at this layer) AND for any team the subject belongs to
// (`team_memberships` - a service_account is never a team member, so
// that subquery is naturally empty for one, not a special case).
//
// API key scope intersection (docs/architecture/13-module-identity-
// rbac-tenancy.md §2: "scopes - optional narrowing below the owner's
// own RBAC grants") happens first, before any DB round-trip: if the
// request authenticated via an API key with a real (non-empty) Scopes
// list (principal.ScopesFromContext, set by httpserver.RequireAuth) and
// permission isn't in it, this returns false immediately - no RBAC
// grant, however broad, can widen back past a key's own narrower scope.
// A JWT-authenticated request, or an API key with an empty Scopes list,
// carries no restriction here and this check is a pure no-op, exactly
// the behavior every existing caller already had before API keys could
// narrow anything.
func (r *RoleBindingRepository) HasPermissionAtScope(ctx context.Context, organizationID, subjectID, permission string, projectID, workspaceID *string) (bool, error) {
	if scopes, ok := principal.ScopesFromContext(ctx); ok && !slices.Contains(scopes, permission) {
		return false, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var allowed bool
	err = tx.QueryRow(ctx, `
		WITH matches AS (
			SELECT rb.effect FROM role_bindings rb
			JOIN roles r ON r.id = rb.role_id
			WHERE rb.organization_id = $1
			  AND (
			    (rb.subject_type IN ('user', 'service_account') AND rb.subject_id = $2)
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
		)
		SELECT EXISTS (SELECT 1 FROM matches WHERE effect = 'allow')
		   AND NOT EXISTS (SELECT 1 FROM matches WHERE effect = 'deny')`,
		organizationID, subjectID, projectID, workspaceID, permission,
	).Scan(&allowed)
	if err != nil {
		return false, err
	}

	return allowed, tx.Commit(ctx)
}

// HasScopedPermission is HasPermissionAtScope's narrower sibling - used
// for real project-visibility gating (Tenancy/Workspace/Execution/
// Variables' new VisibilityChecker port), NOT for the write-permission
// checks HasPermissionAtScope already serves. The difference is
// deliberate: HasPermissionAtScope treats an organization-scope binding
// as matching every Project/Workspace unconditionally (a documented
// invariant every existing caller - create_workspace.go, trigger_run.go,
// cancel_run.go - correctly relies on), but every org member is
// auto-bound to the builtin "read" role AT ORGANIZATION SCOPE on join
// (AddMemberService), which already includes project:read/workspace:read
// - reusing HasPermissionAtScope for a "is this Project even visible to
// this user" check would make nothing actually hidden, since that
// org-scope grant already matches every project. This method drops the
// organization-scope OR-branch entirely: only a binding whose own
// scope_type/scope_id EXACTLY matches the scope passed in counts - a
// Team or User must be explicitly granted at that specific Project (or
// Workspace) to see it. Same subject-matching (direct + team_memberships)
// and same Allow-exists-AND-no-Deny evaluation as HasPermissionAtScope,
// same API-key-scope short-circuit - only the scope predicate differs.
func (r *RoleBindingRepository) HasScopedPermission(ctx context.Context, organizationID, subjectID, permission, scopeType, scopeID string) (bool, error) {
	if scopes, ok := principal.ScopesFromContext(ctx); ok && !slices.Contains(scopes, permission) {
		return false, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return false, err
	}

	var allowed bool
	err = tx.QueryRow(ctx, `
		WITH matches AS (
			SELECT rb.effect FROM role_bindings rb
			JOIN roles r ON r.id = rb.role_id
			WHERE rb.organization_id = $1
			  AND (
			    (rb.subject_type IN ('user', 'service_account') AND rb.subject_id = $2)
			    OR (rb.subject_type = 'team' AND rb.subject_id IN (
			          SELECT team_id FROM team_memberships WHERE user_id = $2 AND organization_id = $1
			    ))
			  )
			  AND rb.scope_type = $3
			  AND rb.scope_id = $4
			  AND r.permissions @> to_jsonb($5::text)
		)
		SELECT EXISTS (SELECT 1 FROM matches WHERE effect = 'allow')
		   AND NOT EXISTS (SELECT 1 FROM matches WHERE effect = 'deny')`,
		organizationID, subjectID, scopeType, scopeID, permission,
	).Scan(&allowed)
	if err != nil {
		return false, err
	}

	return allowed, tx.Commit(ctx)
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
		`INSERT INTO role_bindings (id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, effect, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (role_id, subject_type, subject_id, scope_type, scope_id) DO NOTHING`,
		binding.ID, binding.OrganizationID, binding.RoleID, binding.SubjectType, binding.SubjectID, binding.ScopeType, binding.ScopeID, binding.Effect, binding.CreatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Delete backs DeleteRoleBindingService - a real, permanent removal.
// GRANT DELETE on this table already exists (migrations/0009_grant_
// role_bindings_delete.up.sql), same set_config(...)-scoped-transaction
// pattern as every other write in this file.
func (r *RoleBindingRepository) Delete(ctx context.Context, organizationID, bindingID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM role_bindings WHERE id = $1 AND organization_id = $2`, bindingID, organizationID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DeleteForSubject removes every RoleBinding granted TO a given subject
// (a Team or User being deleted/removed) - backs Tenancy's own
// RoleBindingCleaner port (internal/tenancy/application/ports.go),
// called by DeleteTeamService/RemoveMemberService before the subject
// itself is gone, so no dangling grant is left pointing at a
// nonexistent Team or a User no longer in this org. subject_id has no
// DB-level FK into teams/users (polymorphic, app-validated only, same
// reasoning role_bindings.scope_id already has) - this is the one place
// that cleanup actually happens.
func (r *RoleBindingRepository) DeleteForSubject(ctx context.Context, organizationID, subjectType, subjectID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, organizationID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`DELETE FROM role_bindings WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3`,
		organizationID, subjectType, subjectID,
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
		rows, err = tx.Query(ctx, `SELECT id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, effect, created_at FROM role_bindings WHERE organization_id = $1 ORDER BY created_at`, organizationID)
	} else {
		rows, err = tx.Query(ctx, `SELECT id, organization_id, role_id, subject_type, subject_id, scope_type, scope_id, effect, created_at FROM role_bindings WHERE organization_id = $1 AND subject_id = $2 ORDER BY created_at`, organizationID, subjectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []*domain.RoleBinding
	for rows.Next() {
		var b domain.RoleBinding
		if err := rows.Scan(&b.ID, &b.OrganizationID, &b.RoleID, &b.SubjectType, &b.SubjectID, &b.ScopeType, &b.ScopeID, &b.Effect, &b.CreatedAt); err != nil {
			return nil, err
		}
		bindings = append(bindings, &b)
	}

	return bindings, rows.Err()
}
