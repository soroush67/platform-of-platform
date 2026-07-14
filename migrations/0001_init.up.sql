-- Walking skeleton: Tenancy + Identity + RBAC tables, per
-- docs/architecture/05-database.md §2 (table map) and
-- docs/architecture/03-domain-model.md §2-4 (field lists, invariants).
-- Target: CockroachDB (gen_random_uuid() is a core builtin here, no
-- pgcrypto extension needed - verified against a real node before writing
-- this, not assumed from Postgres familiarity).

-- The non-superuser role every runtime query runs as. Migrations
-- themselves run as root (which implicitly bypasses RLS, verified against
-- a real node), so this schema's RLS policies below only actually
-- constrain anything once the app connects as this role instead of root.
CREATE USER IF NOT EXISTS platform_app;

-- Tenancy context --------------------------------------------------------

CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    slug       text NOT NULL UNIQUE,
    settings   jsonb NOT NULL DEFAULT '{}',
    quota      jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Root of the tenancy hierarchy still gets RLS: without it, a
-- non-superuser app role could list every other org's row by id even
-- though it can't reach anything beneath them.
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
CREATE POLICY organizations_isolation ON organizations
    USING (id = current_setting('app.current_org_id', true)::uuid);

-- Identity & Access context ----------------------------------------------
-- User is platform-global (docs/architecture/03-domain-model.md §3) -
-- no organization_id, no RLS.

CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username     text NOT NULL UNIQUE,
    email        text NOT NULL UNIQUE,
    auth_source  text NOT NULL CHECK (auth_source IN ('local', 'oidc', 'saml', 'ldap')),
    external_id  text,
    status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    mfa_enrolled boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organization_memberships (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    user_id         uuid NOT NULL REFERENCES users (id),
    joined_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, user_id)
);

ALTER TABLE organization_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY organization_memberships_isolation ON organization_memberships
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

-- RBAC context -------------------------------------------------------------
-- organization_id IS NULL means a platform built-in role (Owner/Admin/
-- Write/Read), visible from every org - docs/architecture/03-domain-model.md §4.

CREATE TABLE roles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid REFERENCES organizations (id),
    name            text NOT NULL,
    permissions     jsonb NOT NULL DEFAULT '[]',
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Partial unique indexes instead of a plain UNIQUE(organization_id, name):
-- Postgres treats NULLs as distinct in a regular unique constraint, which
-- would let two different built-in roles both be named "owner".
CREATE UNIQUE INDEX roles_builtin_name_unique ON roles (name) WHERE organization_id IS NULL;
CREATE UNIQUE INDEX roles_org_name_unique ON roles (organization_id, name) WHERE organization_id IS NOT NULL;

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE ROW LEVEL SECURITY;
CREATE POLICY roles_isolation ON roles
    USING (organization_id IS NULL OR organization_id = current_setting('app.current_org_id', true)::uuid);

-- role_bindings.scope_id is a discriminated-union pointer (organization_id
-- | project_id | workspace_id), not a real FK - Postgres can't
-- FK-constrain a polymorphic column (docs/architecture/05-database.md
-- table map, RBAC row). organization_id is carried explicitly (denormalized)
-- so RLS has a real tenant column to filter on regardless of scope_type -
-- this also directly enforces the domain invariant that a binding's scope
-- must be within the same org as the Role it references
-- (docs/architecture/03-domain-model.md §4).
CREATE TABLE role_bindings (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id),
    role_id         uuid NOT NULL REFERENCES roles (id),
    subject_type    text NOT NULL CHECK (subject_type IN ('user', 'team', 'service_account')),
    subject_id      uuid NOT NULL,
    scope_type      text NOT NULL CHECK (scope_type IN ('organization', 'project', 'workspace')),
    scope_id        uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (role_id, subject_type, subject_id, scope_type, scope_id)
);

-- The exact lookup shape RBAC evaluation does on every request
-- (docs/architecture/05-database.md table map, RBAC row).
CREATE INDEX role_bindings_subject_scope ON role_bindings (subject_type, subject_id, scope_type, scope_id);

ALTER TABLE role_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY role_bindings_isolation ON role_bindings
    USING (organization_id = current_setting('app.current_org_id', true)::uuid);

-- Runtime grants for the non-superuser app role - deliberately no
-- DELETE/UPDATE anywhere audit-relevant would go (none of these five
-- tables are the audit_entries table itself, but the same append-mostly
-- posture from docs/architecture/05-database.md's Audit row starts here:
-- grant exactly what this walking skeleton's own code paths need, not a
-- blanket ALL PRIVILEGES).
GRANT SELECT, INSERT, UPDATE ON organizations, users, organization_memberships, roles, role_bindings TO platform_app;
