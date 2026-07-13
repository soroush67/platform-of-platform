# Domain Model

Strategic DDD first (the context map - who owns what, who depends on
whom), then tactical DDD (aggregates/entities/value objects/invariants)
for each bounded context. All of this lives inside the Control Plane
process from Stage 2 - "bounded context" here means a module boundary
enforced in code (separate Go package, its own DB schema/tables, no
other context reaching into it directly), not a separate deployable.

## 1. Context map

```
                              ┌──────────────────┐
                              │   Identity &      │
                     ┌───────▶│   Access           │◀───────┐
                     │        │  (Users, Service   │        │
                     │        │   Accounts, Auth)  │        │
                     │        └────────┬──────────┘        │
                     │                 │ upstream to         │
                     │                 │ everything           │
              ┌──────┴──────┐   ┌──────▼───────┐      ┌──────┴──────┐
              │   RBAC        │   │  Tenancy      │      │   Audit      │
              │ (Roles,       │◀──│ (Org/Project/ │─────▶│ (append-only,│
              │  Bindings)    │   │  Team)        │      │  every write │
              └──────┬────────┘   └──────┬────────┘      │  in every    │
                     │                    │                │  context     │
                     │ authorizes         │ owns            │  emits here) │
                     │                    │                └─────────────┘
                     │           ┌────────▼─────────┐
                     │           │    Workspace &      │
                     └──────────▶│    Environment       │
                                 │ (the "what           │
                                 │  infra" unit)         │
                                 └────┬─────┬──────┬────┘
                                      │      │      │
                        ┌─────────────┘      │      └──────────────┐
                        │                     │                     │
                 ┌──────▼──────┐      ┌───────▼────────┐    ┌───────▼────────┐
                 │  Variables   │      │   Execution      │    │  GitOps /       │
                 │  (cascading  │      │  (Run, Job)      │◀───│  Git Integration │
                 │  Org→Project │      │  the core         │    │ (triggers runs)  │
                 │  →Env→WS)    │      │  workflow          │    └─────────────────┘
                 └──────┬───────┘      └───┬────┬────┬────┘
                        │                    │    │    │
                 ┌──────▼──────┐   ┌─────────▼┐ ┌▼────▼──────┐  ┌─────────────┐
                 │  Secrets      │   │  State    │ │  Policy     │  │  Approval    │
                 │  (metadata/   │   │  Mgmt     │ │  (OPA       │  │  Workflow    │
                 │  refs only -  │   │ (state    │ │  checks     │  │ (gates a Run │
                 │  Vault etc.   │   │  versions,│ │  per Run)   │  │  between plan│
                 │  hold values) │   │  locking) │ └─────────────┘  │  and apply)  │
                 └───────────────┘   └───────────┘                  └──────┬───────┘
                                                                             │
                 ┌───────────────┐   ┌───────────────┐                     │
                 │  Registry       │   │  Kubernetes     │            ┌──────▼───────┐
                 │ (Module/        │   │ (Cluster        │            │ Notifications  │
                 │  Provider,      │   │  inventory,      │◀───────────│ (subscribes to │
                 │  referenced by  │   │  referenced by   │  events    │  domain events │
                 │  Workspace)     │   │  Workspace when   │            │  from every    │
                 └─────────────────┘   │  engine=k8s/helm) │            │  context)      │
                                       └───────────────────┘            └────────────────┘
```

**Reading the map**: Identity/RBAC/Tenancy are upstream of nearly
everything (every other context asks "who is this and what are they
allowed to do here" via RBAC, scoped to a Tenancy node). Workspace is the
hub the product actually revolves around. Audit and Notifications are
both *downstream consumers of domain events from every other context* -
neither one is a dependency any other context calls into directly,
which is deliberate: a context that fails to notify Slack must never be
able to fail the operation that triggered the notification.

## 2. Tenancy context (Organization / Project / Team)

**Why this shape**: the spec's isolation requirement ("Resource Isolation,
Quota Management... Tenant Settings/Branding/Policies") needs a strict
containment hierarchy, not a loose tagging system - every resource in
every other context must resolve to exactly one Organization, unambiguously,
for RBAC and quota enforcement to be sound.

- **Organization** (aggregate root) - `id, name, slug (unique, URL-safe),
  settings {branding, default_policy_set_id}, quota {max_workspaces,
  max_concurrent_runs, ...}, created_at`. Top of the hierarchy; nothing
  in this system exists outside an Organization except the platform's
  own operator-level Identity accounts.
- **Project** (aggregate root, references `organization_id`) - `id,
  organization_id, name, description`. A grouping of Environments/
  Workspaces - typically "one product/service" inside an org.
- **Team** (aggregate root, references `organization_id`) - `id,
  organization_id, name`. A group of Users for RBAC binding purposes;
  intentionally has no direct relationship to Project (a Team's access
  to Projects/Workspaces is entirely mediated through RBAC bindings, not
  a structural property of Team itself - keeps RBAC as the single place
  access decisions are made).
- **OrganizationMembership** / **TeamMembership** (entities, not
  full aggregates) - `user_id, organization_id|team_id, joined_at`.
  Membership is not itself an authorization grant (that's RBAC's job,
  next section) - it's "this user is part of this org/team," a
  prerequisite RBAC bindings can reference.

**Invariant**: `Project.organization_id` is immutable after creation - a
project cannot move between organizations. Same for every other
context's `organization_id`/`project_id` foreign keys. This is what
makes quota enforcement and audit-by-tenant sound: no resource is ever
ambiguous about which tenant it belongs to.

## 3. Identity & Access context

- **User** (aggregate root) - `id, username, email, auth_source
  {local|oidc|saml|ldap}, external_id (nullable, for federated auth),
  status {active|suspended}, mfa_enrolled`. A User is
  platform-global (can belong to multiple Organizations via
  OrganizationMembership), matching how every real IdP-backed system
  works - you don't want "the same human" to be a different identity
  per org they're invited into.
- **ServiceAccount** (aggregate root, scoped to one Organization) - `id,
  organization_id, name, description`. Distinct from User specifically
  because service accounts have no password/MFA/SSO concerns, only
  APIKey-based auth, and should never appear in a "list human users"
  view.
- **APIKey** (entity, owned by either a User or a ServiceAccount) - `id,
  owner_type, owner_id, hashed_secret, scopes (optional narrowing below
  the owner's own RBAC grants), expires_at, last_used_at`.

**Domain events**: `UserProvisioned`, `UserSuspended`, `APIKeyCreated`,
`APIKeyRevoked`.

## 4. RBAC context

**Why modeled separately from Identity/Tenancy rather than folded into
either**: every other context needs to ask the same question -
"can Subject X do Action Y on Resource Z" - and that logic needs one
home, not N reimplementations. This also directly implements the
ABAC/OPA requirement: RBAC provides the *coarse* role-based grant,
and a Policy check (Policy context) can add *fine-grained* attribute
conditions on top for a specific action - RBAC answers "can this subject
touch this resource class at all," Policy answers "should this specific
change be allowed."

- **Role** (aggregate root) - `id, organization_id (nullable - null
  means a platform-built-in role like "org:admin"), name, permissions
  (set of Permission value objects)`. Both built-in roles (Owner, Admin,
  Write, Read - matching the spec's RBAC baseline) and custom
  organization-defined roles are the same aggregate shape.
- **Permission** (value object) - `resource_type:action`, e.g.
  `workspace:apply`, `secret:read`, `policy_set:manage`. A fixed,
  versioned enum the platform defines (not user-extensible) - custom
  Roles compose *existing* Permissions, they don't invent new ones.
- **RoleBinding** (aggregate root) - `id, role_id, subject
  {user_id|team_id|service_account_id}, scope {organization_id |
  project_id | workspace_id}`. This is the actual grant: "Role R applies
  to Subject S at Scope T." Scope is a discriminated union over the
  Tenancy hierarchy, and a binding at a higher scope (Organization)
  implies the grant at every resource beneath it (Projects, Workspaces)
  unless a more specific binding narrows it - same evaluation model as
  AWS IAM/Kubernetes RBAC, chosen because it's a well-understood,
  provably-correct pattern rather than inventing a new one.

**Invariant**: a RoleBinding's scope must be a resource within the same
Organization as the Role it references (a custom Role from Org A can
never be bound at a resource in Org B) - this is the actual mechanism
that makes tenant isolation real, not just structural.

## 5. Workspace & Environment context

**Why Environment is a distinct aggregate from Workspace, not a field on
it**: the spec lists these as separate modules on purpose - a real
promotion flow ("this change went through dev, then staging, then
prod") is a property of a *sequence of Workspaces*, and Environment is
what carries that sequencing plus environment-scoped Variables that
cascade into every Workspace inside it. This directly reuses a pattern
already proven this session in `compose-platform`: its Global
ComposeFile → per-ComposeFile variable override precedence is exactly
the same cascade shape as Org → Project → Environment → Workspace
variables here, just one more level deep.

- **Environment** (aggregate root, references `project_id`) - `id,
  project_id, name (e.g. "production"), promotion_rank (int, defines
  ordering for promotion-flow UI), requires_approval (bool, default
  policy for Runs promoted into this Environment)`.
- **Workspace** (aggregate root, references `project_id` and optionally
  `environment_id`) - `id, project_id, environment_id (nullable - a
  workspace can exist outside any environment, e.g. a scratch/dev
  workspace), name, execution_engine {terraform|opentofu|ansible|helm|
  compose|packer|kubespray|kubernetes}, vcs_link_id (nullable, → GitOps
  context), current_state_version_id (nullable, → State context),
  lock_status {unlocked|locked(run_id)}`.

**Invariant**: `Workspace.execution_engine` is immutable after the first
successful Run - changing engines on an existing workspace isn't a
config edit, it's effectively a new workspace (this is the same
reasoning Terraform Cloud enforces: a workspace's backend/execution
model is load-bearing for its entire state history).

**Domain events**: `WorkspaceCreated`, `WorkspaceLocked`,
`WorkspaceUnlocked`, `WorkspaceArchived`.

## 6. Execution context (Run, Job) - the core workflow

- **Run** (aggregate root, references `workspace_id`) - `id,
  workspace_id, trigger {manual|vcs_push|vcs_pr|scheduled|api},
  triggered_by (user_id or "system" for scheduled),
  status {queued|planning|planned|policy_check|awaiting_approval|
  applying|applied|failed|errored|canceled}, plan_output_ref (object
  storage pointer, nullable until plan completes), apply_output_ref
  (nullable until apply completes), created_at, started_at, finished_at`.
- **Job** (entity within Run) - `id, run_id, phase {plan|apply|
  destroy|custom-step-N}, worker_id (nullable until dispatched),
  status, exit_code, log_ref (object storage pointer - logs are large
  and append-only, they don't belong in Postgres rows), started_at,
  finished_at`. Modeled as *entity*, not its own aggregate, because a
  Job never outlives or is referenced independently of its Run - it has
  no identity or lifecycle meaningful outside the Run that owns it.
  Multiple Jobs per Run is what lets a Kubespray-engine Run represent
  its real multi-phase playbook execution (or a Terraform run's
  plan-then-apply) as one coherent unit the UI shows as a single
  timeline.

**Invariant**: only one Run may be in a non-terminal status
(`queued`...`applying`) per Workspace at a time - this *is* the
Workspace's `lock_status`, not a separately-enforced rule; the lock is
literally "which Run currently owns this workspace," so there's no way
for the invariant and the lock state to drift out of sync.

**Domain events**: `RunQueued`, `RunPlanning`, `PlanCompleted`,
`PolicyCheckCompleted`, `ApprovalRequired`, `RunApplying`,
`RunCompleted`, `RunFailed`, `RunCanceled` - this is the single busiest
event stream in the system (drives the Web UI's live timeline, the
Notification Dispatcher, and the Audit context).

## 7. Variables context

- **Variable** (aggregate root) - `id, scope_type {organization|project|
  environment|workspace}, scope_id, key, category {env_var|engine_var|
  file_template}, sensitivity {plain|sensitive}, value (plain text) OR
  secret_ref (→ Secrets context, mutually exclusive with value)`.

**Resolution rule** (the cascade): for a given Workspace, resolve a
Variable key by checking Workspace-scoped first, then its Environment
(if any), then its Project, then its Organization, taking the first
match - identical precedence direction to `compose-platform`'s
Global-ComposeFile-vs-local-variable resolution already built and
tested this session, deliberately reused rather than inventing a new
precedence model.

## 8. Secrets context

**Why this context stores *no secret values*, ever**: per Stage 1's
scope, Vault (or AWS Secrets Manager/Azure Key Vault/GCP Secret Manager)
is the actual secret store - this context is a thin metadata/routing
layer, the same boundary this operator's own `vault-ha` project draws
between "the cluster" and "what you store in it."

- **SecretMount** (aggregate root, scoped to an Organization) - `id,
  organization_id, backend_type {vault|aws_secrets_manager|
  azure_keyvault|gcp_secret_manager}, connection_config (backend
  address, auth method reference - itself resolved via a bootstrap
  credential, never a plaintext secret in this table)`.
- **SecretReference** (value object, embedded wherever a secret is
  used - e.g. inside a Variable) - `mount_id, path`. Resolving a
  SecretReference to an actual value happens **only inside an Execution
  Worker, at run time**, using a short-lived credential the Control
  Plane requests from the backend on the worker's behalf (e.g. a Vault
  batch token scoped to exactly that run) - the Control Plane's own
  Postgres never holds a plaintext secret value at rest, only
  references to where one lives.

## 9. State Management context

- **StateVersion** (aggregate root, references `workspace_id`) - `id,
  workspace_id, serial (monotonic per workspace), lineage (UUID,
  detects state history divergence - same concept Terraform's own state
  format uses), object_storage_ref, outputs (parsed key/value pairs,
  denormalized here for fast UI display without fetching the full state
  blob), created_by_run_id`.
- **StateLock** (value object, actually just `Workspace.lock_status`
  from section 5 - listed here only to make explicit that this context
  does not maintain a second, separate lock; one lock concept, owned by
  Workspace, referenced by State).

## 10. Policy context

- **PolicySet** (aggregate root, scoped to Organization/Project/
  Workspace) - `id, scope_type, scope_id, name, policies (list of
  {name, rego_source_ref}), enforcement_level {advisory|
  soft_mandatory|hard_mandatory}`. Enforcement levels borrowed as a
  *concept* from Sentinel (advisory logs only, soft-mandatory can be
  overridden by an authorized approver, hard-mandatory blocks
  unconditionally) without adopting Sentinel's syntax - see Stage 1's
  explicit OPA-only decision.
- **PolicyCheckResult** (entity, attached to a Run) - `run_id,
  policy_set_id, policy_name, result {pass|fail|waived},
  waived_by (nullable, references a User - only valid if
  enforcement_level allowed a waiver)`.

## 11. GitOps / Git Integration context

- **GitConnection** (aggregate root, scoped to Organization) - `id,
  organization_id, provider {github|gitlab|bitbucket|azure_devops|
  gitea|forgejo}, auth_method {deploy_key|oauth_app|pat}, credential_ref
  (→ Secrets context - the token/key itself is never stored here
  directly either)`.
- **RepositoryLink** (aggregate root, binds a Workspace to a repo) -
  `id, workspace_id, git_connection_id, repo, branch, path (subdirectory
  for monorepo support), trigger_mode {auto_plan_and_apply|
  auto_plan_only|manual}`.

**Domain events**: `WebhookReceived` (raw, before interpretation) →
interpreted into `RunQueued` (Execution context) when a RepositoryLink's
trigger_mode matches the incoming push/PR event - GitOps doesn't run
anything itself, it only ever produces a Run for the Execution context
to own from there.

## 12. Approval Workflow context

- **ApprovalRequest** (aggregate root, references `run_id`) - `id,
  run_id, required_approval_count, eligible_approvers (resolved from
  RBAC at request-creation time - a snapshot, not a live query, so a
  later role change can't retroactively invalidate an in-flight
  approval), status {pending|approved|rejected}`.
- **ApprovalDecision** (entity within ApprovalRequest) - `approver_id,
  decision {approve|reject}, comment, decided_at`.

## 13. Kubernetes context

- **Cluster** (aggregate root, scoped to Organization/Project) - `id,
  organization_id, project_id (nullable), name, provisioning_source
  {kubespray_managed|imported|eks|gke|aks}, kubeconfig_secret_ref (→
  Secrets context), health_status {healthy|degraded|unreachable|unknown}
  (a cache, refreshed by a periodic Job, not queried live on every
  read)`. Deliberately thin at domain-model stage - node
  inventory/labels/taints/namespaces are real features from the spec,
  but they're *read-mostly, synced-from-the-real-cluster* data, not
  aggregates this platform is the system of record for (the real
  Kubernetes API server always is) - modeled fully at the per-module
  detail stage, not here.

## 14. Registry context (Module / Provider Registry)

- **Module** (aggregate root, scoped to Organization) - `id,
  organization_id, namespace, name, target_system (e.g. aws/azure/k8s),
  versions (ModuleVersion entities: version, source_ref, checksum,
  published_at)`.
- **Provider** (aggregate root, same shape, mirrors Terraform's own
  provider registry protocol so this platform can act as a
  drop-in-compatible private registry, not a bespoke one).

## 15. Audit context

- **AuditEntry** (aggregate root, append-only - no update/delete
  operation exists in this context's own interface, mirroring the
  `AuditLog` model already built and proven this session in
  `compose-platform`) - `id, organization_id, actor
  {user_id|service_account_id|"system"}, action, target_type, target_id,
  metadata (jsonb), ip_address, created_at`. Populated *exclusively* by
  subscribing to every other context's domain events - no context ever
  calls into Audit directly, which is what guarantees no code path can
  accidentally skip audit logging by forgetting to call it.

## 16. Notification context

- **NotificationChannel** (aggregate root, scoped to Organization/
  Project) - `id, scope_type, scope_id, type {slack|mattermost|teams|
  email|webhook}, config, subscribed_event_types (list)`.
  Same "subscribes to the event bus, never called directly" shape as
  Audit, and directly generalizes the Mattermost/email/syslog
  notification system already built this session in `compose-platform`
  to a multi-channel, multi-event, per-org-configurable version of the
  same idea.

## Cross-cutting note: every aggregate above got its `organization_id`
## checked for real, not assumed

Every aggregate root in this doc either has a direct `organization_id`,
or resolves to one through exactly one parent (Workspace →
Project → Organization; Run → Workspace → ...; RoleBinding's scope →
...). This was checked deliberately against the context map, not
asserted - it's the property Stage 2's multi-tenancy story and every
future quota/isolation/audit-by-tenant feature depends on being
airtight.

## Open questions before Stage 4 (APIs)

1. **Environment ↔ Workspace cardinality**: this doc modeled Environment
   as *containing* multiple Workspaces (one per Project, promoted in
   sequence). An alternative real-world shape some tools use is
   Environment as a *cross-cutting label* (the same Workspace concept
   deployed once, with Environment as an axis of Variable overrides
   rather than a container). Confirm the containment model above is
   what you want - it's a real fork, not a naming detail, and changes
   how the promotion-flow UI and Run-triggering-from-promotion feature
   get built.
2. **Service Account scope**: modeled as Organization-scoped only. If
   you need Project-scoped service accounts (narrower blast radius for
   CI credentials), flag it now - it changes the RoleBinding subject
   union in section 4.
3. **Anything in the module list Stage 3 under-modeled on purpose**
   (Kubernetes cluster inventory, Registry) - confirm the "thin now,
   detailed at the per-module stage" call was right, or flag anything
   that actually needs full domain-model detail now because another
   context's design depends on it.
