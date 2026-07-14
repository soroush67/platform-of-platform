-- Closes three of the four named RBAC gaps this slice targets:
--   1. Owner vs Admin real differentiation (organizations.status/archived_at
--      + a new organization:delete permission only Owner gets - see
--      internal/rbac/domain/role.go).
--   2. Team aggregate (docs/architecture/03-domain-model.md §2) -
--      role_bindings.subject_type already allowed 'team' at the schema
--      level since migration 0001, but no teams table ever existed to
--      reference.
-- The fourth gap (custom roles, RoleBinding at project/workspace scope)
-- needs no schema change at all - migration 0001's roles/role_bindings
-- tables already support organization_id-scoped custom roles and
-- scope_type IN ('organization','project','workspace'); only the
-- domain/application/adapter code was restricted to the organization-only
-- subset until this slice.

ALTER TABLE organizations
    ADD COLUMN status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    ADD COLUMN archived_at timestamptz;

CREATE TABLE teams (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    name            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams FORCE ROW LEVEL SECURITY;
CREATE POLICY teams_isolation ON teams
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

CREATE TABLE team_memberships (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         uuid NOT NULL REFERENCES teams (id),
    -- Denormalized, not derived via a join to teams on every query -
    -- same reasoning as role_bindings.organization_id: RLS policies need
    -- a column to filter on directly, without a subquery, per
    -- docs/architecture/05-database.md §1's own established pattern.
    organization_id uuid NOT NULL REFERENCES organizations (id),
    user_id         uuid NOT NULL REFERENCES users (id),
    joined_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, user_id)
);

-- The exact lookup HasPermission's team-mediated join needs (see
-- role_binding_repository.go): "which teams is this user in."
CREATE INDEX team_memberships_user ON team_memberships (user_id);

ALTER TABLE team_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY team_memberships_isolation ON team_memberships
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

-- organizations already has SELECT/INSERT/UPDATE granted (0001_init) -
-- the new status/archived_at columns need no additional GRANT.
GRANT SELECT, INSERT, UPDATE ON teams, team_memberships TO platform_app;
