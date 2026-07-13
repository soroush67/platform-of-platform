# Module detail: Identity & Access, RBAC, Tenancy

Seventh module doc, three contexts together per Stage 12's closing
recommendation - referenced from literally every other module doc so
far (every endpoint's authorization check resolves through here) but
never given its own endpoint-by-endpoint surface. This doc is mostly
mechanical at this point precisely because the *interesting* design
work (the RLS isolation model, the scoped-RoleBinding evaluation, the
Permission-enum-as-scope-for-API-keys decision) already happened in
Stages 3-5 - this doc wires it to concrete endpoints.

## 1. REST API - Organization / Project / Team (Tenancy)

```
POST   /api/v1/orgs                              (platform-level operation - see §4 on who can do this)
GET    /api/v1/orgs/{org}
PATCH  /api/v1/orgs/{org}                          name, settings, quota (quota: platform-admin only, see §4)
POST   /api/v1/orgs/{org}/projects
GET    /api/v1/orgs/{org}/projects/{project}
POST   /api/v1/orgs/{org}/teams
POST   /api/v1/orgs/{org}/teams/{team}/members     { "user_id": "uuid" }
DELETE /api/v1/orgs/{org}/teams/{team}/members/{user_id}
POST   /api/v1/orgs/{org}/members                  invite/add a User to the Organization directly (OrganizationMembership, Stage 3 §2 - independent of Team membership)
```

Nothing here beyond standard resource CRUD - flagged specifically
*because* it's unremarkable: the hard problems (isolation, cascading
delete behavior) were solved at the schema level in Stage 5, not
something this API layer needs to re-solve.

**One real rule worth stating**: deleting an Organization is **not** a
hard `DELETE` - `DELETE /orgs/{org}` sets a `status: archived` flag and
schedules a background purge job 30 days out (configurable), the same
"soft-delete with a grace period, not instant destruction" posture this
operator's own `compose-platform` already applies to Users/Machines
with audit history (Stage 3's Audit context makes this doubly
important here: an instantly-deleted Organization would orphan every
`organization_id` foreign key that RLS and Audit both depend on being
resolvable).

## 2. REST API - User / ServiceAccount / APIKey (Identity)

```
GET    /api/v1/users/me                            the authenticated caller's own profile
PATCH  /api/v1/users/me                             self-service profile edits only
POST   /api/v1/orgs/{org}/service-accounts
POST   /api/v1/orgs/{org}/service-accounts/{sa}/api-keys
  { "scopes": ["workspace:plan", "workspace:apply"], "expires_at": "..." }
  → 201, returns the plaintext key **exactly once** - same "shown once,
    never again, nothing server-side ever holds the plaintext" posture
    as this operator's own vault-ha project's unseal-key handling this
    session, applied here to API keys instead of Shamir shares.
DELETE /api/v1/orgs/{org}/service-accounts/{sa}/api-keys/{key}   revoke
```

There is deliberately **no** `PATCH` on an APIKey's scopes after
creation - narrowing or widening a live credential's access in place is
exactly the kind of change that's easy to lose track of ("wait, when
did this CI key get apply access") - the only path to changing scopes
is revoke-and-reissue, which forces a deliberate, auditable,
new-credential-distribution act instead of a silent widening.

## 3. REST API - Role / RoleBinding (RBAC)

```
GET    /api/v1/orgs/{org}/roles                    lists built-in + org-custom roles
POST   /api/v1/orgs/{org}/roles                     custom role: { name, permissions: [...] }
POST   /api/v1/orgs/{org}/role-bindings
  { "role_id": "...", "subject": {"type": "user", "id": "..."},
    "scope": {"type": "workspace", "id": "..."} }
GET    /api/v1/orgs/{org}/role-bindings?subject_id=...    "what can this user do, and where"
```

The last endpoint is the concrete answer to a question every other
module doc has implicitly assumed is answerable: the Web UI's
"why can't I do this" and an admin's "what does this person have access
to" both resolve through one query, not a bespoke permissions-explainer
per context.

**Built-in roles** (seeded at platform install, `organization_id: null`
per Stage 3 §4, exist identically in every org): `owner` (every
Permission), `admin` (every Permission except billing/org-deletion),
`write` (plan/apply/create-workspace-level actions, no RBAC/policy
management), `read` (read-only everywhere). Custom roles compose the
same fixed Permission enum (Stage 3 §4 already ruled out user-defined
Permissions) - this doc doesn't add new built-ins beyond what a
Terraform-Cloud-shaped product's baseline needs, deliberately minimal
rather than a large starter set that's mostly guessing at what
organizations will actually want.

## 4. Platform-level operations - the one place this doc introduces
## something not yet covered: who can create an Organization at all

Every endpoint elsewhere in this doc set assumes an existing
Organization scope to check RBAC against - `POST /orgs` itself can't,
since there's no Organization yet to bind a Role at. **Platform
Administrator** is a distinct concept from every org-scoped Role: a
flag on the User record itself (`platform_admin: bool`), not a
RoleBinding, checked only by the small set of endpoints that operate
above the Organization level (`POST /orgs`, platform-wide quota
defaults, license management from the original spec's list). Kept
structurally separate from the RBAC model in §3 on purpose - conflating
"can manage everything in every org" with "has an org-scoped Role" would
mean either polluting every RoleBinding scope check with a "...or is
platform_admin" special case, or accepting that a sufficiently-scoped
custom Role could accidentally grant platform-wide power. A single
boolean flag, checked in exactly the handful of endpoints that need it,
is simpler and more auditable than folding it into the scoped grant
system §3 already established.

## Open questions before the next module doc

1. **Self-service org creation**: should any authenticated User be
   allowed to create their own Organization (the common self-serve SaaS
   pattern - "sign up, get an org automatically"), or is `POST /orgs`
   platform-admin-only (matches an enterprise-appliance / self-hosted-
   first posture, per Stage 2's v1 target)? This doc left it unresolved
   because it's genuinely a product decision, not an architecture one -
   both are easy to build, the answer changes onboarding UX, not the
   schema.
2. **Last module pair**: Audit + Notifications next, which completes
   all 16 Stage 3 contexts at the module-detail level. After that, the
   process's own plan (Stage 0) moves into UI, Backend, Workers,
   Integrations, Tests, Deployment - worth explicitly confirming that's
   still the right order once all 16 modules are done, or whether
   something learned along the way (e.g., the recurring "lightweight
   Worker job outside the Run state machine" pattern from Kubernetes/
   Registry) deserves its own cross-cutting doc before moving on.
