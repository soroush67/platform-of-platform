# Database

Maps Stage 3's aggregates onto real storage. Four systems, each already
justified in Stage 1/2 (and one, standalone TSDB/search, deliberately
cut) - this doc goes one level deeper: *which* tables, *what* isolation
model, *what* indexes the Stage 4 API's actual access patterns need.

## 0. Engine: CockroachDB, not vanilla Postgres

**Updated from this doc's original draft, which assumed Postgres without
naming the choice explicitly** - CockroachDB, wire- and SQL-compatible
enough with Postgres that everything below (RLS, `pgx`, `golang-migrate`)
carries over unchanged, chosen over vanilla Postgres for one property
vanilla Postgres doesn't have natively: **real horizontal clustering**.
Stage 2's HA story for every other stateless component (Control Plane,
Worker, Notification Dispatcher) is "run more replicas behind a load
balancer" - Postgres alone can't answer the same way for the one
genuinely stateful piece without bolting on a separate HA layer
(Patroni/repmgr + a proxy) that's operational complexity this design
doesn't otherwise need anywhere else. CockroachDB gives that clustering
as a property of the database itself (add a node, it rebalances ranges
automatically), which is the deployment target this system needs -
**this isn't a v1-only dev convenience, real multi-node CockroachDB
clustering is the production HA target**, not something deferred to a
"future managed Postgres" swap-in. It's also the exact database this
operator's own `compose-platform`, built earlier this session, already
runs in production - reusing a proven-in-this-session stack choice
rather than introducing a second database technology to operate.

**Verified for real before committing to this, not assumed from Postgres
familiarity**: against a real single-node CockroachDB container,
`gen_random_uuid()` is a core builtin (no `pgcrypto` extension needed),
`ENABLE ROW LEVEL SECURITY` / `FORCE ROW LEVEL SECURITY` / `CREATE POLICY
... USING (...)` all work, a custom session variable
(`SET app.current_org_id = '...'`) is readable via
`current_setting('app.current_org_id', true)`, and - the property that
actually matters - a non-superuser role scoped to one org via that
session variable **only ever sees that org's rows**, while `root`
(migrations) transparently bypasses RLS the same way a Postgres
superuser does. §1 below is the reasoning; this paragraph is the
confirmation that the reasoning holds on the actual engine chosen, via
`docker exec ... cockroach sql` against org-A/org-B rows and an
`app_user` role, not asserted from how Postgres RLS is known to behave.

## 1. Multi-tenancy isolation: shared schema + `organization_id` + Row-Level Security

Three real options existed: database-per-tenant, schema-per-tenant, or
shared schema with a tenant column. Picking shared-schema-plus-RLS, and
explaining why the other two lose:

- **Database-per-tenant** - operationally the heaviest (connection
  pooling, migrations, and backups all multiply by org count) and
  actively hostile to the "self-hosted single-org" v1 target from
  Stage 2, where "org count" is usually 1 anyway, making the isolation
  benefit moot while still paying the operational cost in the
  multi-tenant SaaS target where it matters more.
- **Schema-per-tenant** - splits the difference badly: still multiplies
  migrations by org count (every schema needs the new column/table
  applied), while giving weaker isolation guarantees than RLS actually
  enforced at the query-planner level (a schema-qualified query is only
  as safe as every single query remembering to qualify it).
- **Shared schema + RLS** (chosen): one migration ever applies to one
  physical schema. Every table carrying tenant data gets an
  `organization_id` column and a `USING (organization_id =
  current_setting('app.current_org_id')::uuid)` RLS policy - the
  Control Plane sets `app.current_org_id` once per request (from the
  authenticated Principal, Stage 4 §4) and **every query, including ones
  a future engineer forgets to hand-filter, is constrained by Postgres
  itself, not by application code discipline**. This is the same
  isolation model Notion, Retool, and most modern multi-tenant Postgres
  SaaS products converge on, for exactly this "isolation enforced by the
  database, not by remembering" reason - and it costs nothing extra for
  the single-org self-hosted case, where there's simply one
  `organization_id` value in play.

## 2. Table map (one row per Stage 3 aggregate/entity; PK is always `id uuid` unless noted)

| Context | Table(s) | Notable columns beyond Stage 3's field list | Indexes driven by Stage 4's API |
|---|---|---|---|
| Tenancy | `organizations`, `projects`, `teams`, `organization_memberships`, `team_memberships` | `organizations.slug` unique; `projects.slug` unique *within org* | `(organization_id, slug)` unique composite on every slugged table |
| Identity | `users`, `service_accounts`, `api_keys` | `users.email` unique globally (Stage 3: User is platform-global) | `(hashed_secret)` unique on `api_keys` for auth lookup |
| RBAC | `roles`, `role_bindings` | `role_bindings.scope_type` + `scope_id` as a discriminated pair, not a polymorphic FK (Postgres can't FK-constrain a polymorphic column - the RBAC evaluation service validates `scope_id` exists in the right table at write time instead) | `(subject_type, subject_id, scope_type, scope_id)` - the exact lookup shape RBAC evaluation does on every request |
| Workspace/Env | `environments`, `workspaces` | `workspaces.lock_status` as `(locked bool, locked_by_run_id uuid nullable)` | `(project_id)`, `(environment_id)` |
| Execution | `runs`, `jobs` | `runs.status` as a Postgres `enum` type (not a free-text column - Stage 3's status set is closed and this makes an invalid status a schema-level impossibility, not just an app-level bug) | `(workspace_id, status, created_at desc)` - the Stage 4 cursor-pagination query's exact shape; partial index `WHERE status IN (running states)` for the "is this workspace locked" hot-path check |
| Variables | `variables` | `(scope_type, scope_id)` same discriminated-pair pattern as RBAC | `(scope_type, scope_id, key)` unique - two variables can't collide at the same scope |
| Secrets | `secret_mounts` | connection_config as `jsonb` (backend-specific shape, not worth a rigid column set) | - |
| State | `state_versions` | `outputs` as `jsonb` (Stage 3 called this out as denormalized-for-fast-display) | `(workspace_id, serial desc)` - "give me the latest state version" is the dominant query |
| Policy | `policy_sets`, `policy_check_results` | - | `(run_id)` on results |
| GitOps | `git_connections`, `repository_links` | - | `(workspace_id)` unique on `repository_links` - Stage 3 implies one VCS link per workspace |
| Approval | `approval_requests`, `approval_decisions` | - | `(run_id)` unique on requests - one approval flow per run |
| Kubernetes | `clusters` | `health_status` + `health_checked_at` (Stage 3 called this a periodically-refreshed cache) | - |
| Registry | `modules`, `module_versions`, `providers`, `provider_versions` | - | `(organization_id, namespace, name)` unique |
| Audit | `audit_entries` | **no `updated_at`, no update/delete privilege granted to the app's DB role at all** - enforced at the database-permission level, not just "the code never calls UPDATE," so a bug (or a compromised app process) still can't rewrite history | `(organization_id, created_at desc)`, `(target_type, target_id)` |
| Notifications | `notification_channels` | - | - |

**What's deliberately NOT a Postgres table**: Run/Job logs (object
storage, per Stage 3 - too large and write-once/read-many for row
storage), state file contents (object storage, only the *metadata* row
lives in `state_versions`), and the workspace lock's *enforcement*
(that's a Postgres `SELECT ... FOR UPDATE` inside the transaction that
transitions a Run into a running status, not a separate lock table -
one less thing that can drift from the `runs` table it's protecting).

## 3. Migrations

**Plain versioned SQL migrations** (via `golang-migrate` or Atlas -
final pick deferred to the Backend stage, both are proven, both fit a
Go stack), **not an ORM's auto-migrate**. Reasoning: this schema has
real invariants (RLS policies, a closed `enum` for `runs.status`,
partial indexes) that an ORM's schema-diffing auto-migrate handles
poorly or not at all - hand-written, reviewed SQL migrations are the
only approach that keeps those invariants a first-class, version-
controlled part of the schema rather than an afterthought bolted on
after the ORM generates its idea of the schema.

## 4. Object storage layout

```
s3://platform-artifacts/
  orgs/{org_id}/
    state/{workspace_id}/{serial}.tfstate.json.zst   (zstd - state files compress extremely well, and workspaces can accumulate hundreds of versions)
    runs/{run_id}/jobs/{job_id}/log.ndjson.zst        (newline-delimited JSON, one line per log event - structured, not raw terminal bytes, so the Web UI can render/filter without re-parsing ANSI codes)
    runs/{run_id}/plan-output.json.zst
  modules/{namespace}/{name}/{version}.tar.zst
  providers/{namespace}/{name}/{version}/{platform}.zip
```

Org-scoped prefix on every tenant-owned object (`orgs/{org_id}/...`) so
a presigned-URL bug that's too permissive on *path* still can't cross an
org boundary if bucket policy also constrains by prefix - defense in
depth alongside the Postgres RLS story above, not a replacement for it.

## 5. Redis usage (cache + coordination, never system-of-record)

- **Live log tail**: `PUBLISH job:{job_id}:log <line>` from the Worker's
  gRPC log stream (Stage 4 §10) as it arrives; the Web UI's backing
  WebSocket handler `SUBSCRIBE`s the same channel. Redis pub/sub is
  fire-and-forget by design here - the *durable* copy of the log is the
  object storage write happening concurrently; a UI viewer who wasn't
  subscribed at the moment a line was published just doesn't see it live
  and reads the full log from object storage instead. No durability
  requirement on this path, which is exactly what makes plain Redis
  pub/sub (not JetStream) the right tool for it.
- **Rate limiting** (Stage 4 §6): token bucket state, `INCR` +
  `EXPIRE`, standard.
- **Response caching**: read-heavy, rarely-changing lookups (e.g.
  "resolve org slug → id" on every single request) - short TTL (seconds),
  explicitly *not* a cache any correctness property depends on (a stale
  hit for a few seconds during a rename is an acceptable, bounded
  inconsistency, not a bug to design around).

## 6. What this doc deliberately deferred

- **Partitioning `runs`/`audit_entries` by time** once they're large
  enough to matter (both are the two tables in this schema with
  genuinely unbounded growth) - real technique, real future need,
  premature to design the exact partition boundary now against data
  that doesn't exist yet. Flagged here so it isn't forgotten, not
  designed here so it isn't designed against guessed numbers.
- **Read replicas / follower reads** - CockroachDB's own answer to this
  (§0) is multi-node clustering plus, if read latency ever justifies it,
  `AS OF SYSTEM TIME follower_read_timestamp()` reads served from the
  nearest replica - a deployment-topology and query-routing concern,
  same as read replicas would be on any engine; which queries actually
  need it is an optimization pass once real query load exists to
  optimize against, not designed here against guessed numbers.

## Open questions before Stage 6 (events)

1. **RLS session variable propagation**: confirms the Control Plane's
   connection-pooling strategy needs to `SET app.current_org_id` per
   request on a pooled connection (not a fresh connection per request) -
   this is a real, slightly tricky bit of Go/pgx wiring (the setting
   must not leak between requests reusing the same pooled connection).
   Flagging now so Backend-stage connection-pool design accounts for it
   from the start rather than discovering it as a cross-tenant-leak bug
   later.
2. Any Stage 3 aggregate you want a **stronger consistency story** for
   than "eventually consistent via the event bus" - the table map above
   assumes every cross-context read (e.g. the Web UI showing a Run's
   Workspace name) either joins within one transaction (same-context
   reads) or accepts the Stage 3 event-driven update lag (cross-context
   reads, e.g. a Notification Channel showing a slightly-stale
   Workspace name). Flag if any specific cross-context view needs to be
   strongly consistent instead - it changes whether that view is served
   by a join or a projection.
