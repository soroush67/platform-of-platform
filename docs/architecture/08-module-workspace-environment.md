# Module detail: Workspace & Environment

Second module doc. Chosen next because Execution's Run Dispatcher (Stage
7 §3) has to resolve a Workspace's variables and lock state before it
can hand a Job to a Worker - this doc makes that resolution algorithm
concrete, and closes Stage 3's open question about Environment↔Workspace
promotion with an actual API instead of just a data model.

## 1. REST API - Workspace

Paths relative to `/api/v1/orgs/{org}/projects/{project}`.

### `POST /workspaces`

```json
{
  "name": "prod-vpc",
  "environment_id": "uuid | null",
  "execution_engine": "terraform"     // immutable after first successful Run, per Stage 3 §5
}
```

### `GET /workspaces`, `GET /workspaces/{workspace}`

Standard resource read; `GET /workspaces/{workspace}` includes
`lock_status` inline (`{locked: bool, locked_by_run_id, locked_at}`) -
this is a field on the Workspace row itself (Stage 5), not a joined
lookup, so it's free to include.

### `PATCH /workspaces/{workspace}`

Only `name`, `environment_id` are mutable. A `PATCH` attempting to
change `execution_engine` after the workspace has a Run in `applied`
status returns `409` with a message pointing at "create a new
workspace" - enforcing Stage 3's immutability invariant at the API
boundary, not just documenting it.

### `POST /workspaces/{workspace}/force-unlock`

Admin-only (RBAC permission `workspace:force_unlock`, deliberately
separate from `workspace:apply` - being allowed to run applies doesn't
imply being allowed to break a lock someone else's in-flight Run
holds). Sets `lock_status` to unlocked *without* the owning Run
reaching a terminal status - this is explicitly an operator escape
hatch for "the Stale Run Reaper hasn't caught this yet and I need to
unblock the team now," and it's audited with extra weight (the Audit
context's `metadata` for this action always includes the Run it forced
past, so "who broke my lock and why" is always answerable).

### `GET /workspaces/{workspace}/variables` (effective, resolved view)

```json
{
  "variables": [
    { "key": "region", "value": "us-east-1", "resolved_from": "environment", "source_id": "env-uuid" },
    { "key": "instance_type", "value": "t3.large", "resolved_from": "workspace", "source_id": "workspace-uuid" },
    { "key": "db_password", "value": "••••••••", "resolved_from": "organization", "source_id": "org-uuid", "sensitive": true }
  ]
}
```

**This is the concrete algorithm behind Stage 3 §7's cascade**, exposed
as a real endpoint rather than only living in the Run Dispatcher's
internal logic - specifically so the Web UI can show a user *why* a
variable has the value it does (which scope it actually resolved from)
without reimplementing the resolution algorithm client-side. Sensitive
values are always masked here (this is a read endpoint reachable by
anyone with `workspace:read`, which is a much wider audience than who
should see plaintext secrets) - the real value only ever gets resolved
inside a Worker, at Job start, per Stage 3 §8's boundary.

## 2. REST API - Environment

Paths relative to `/api/v1/orgs/{org}/projects/{project}`.

### `POST /environments`

```json
{ "name": "production", "promotion_rank": 3, "requires_approval": true }
```

### `POST /environments/{environment}/promote`

**This is the concrete resolution of Stage 3's open question.** Promotion
means: take a specific Run's *inputs* (resolved variable set, module/
config version) from one workspace and apply that same input as a new
Run on the next-ranked workspace in sequence.

```json
// Request
{ "source_run_id": "uuid" }   // must be an `applied` Run on a workspace in a lower-ranked environment
// 202 Accepted
{ "id": "uuid", "status": "queued", "promoted_from_run_id": "source-run-uuid" }
```

Server-side: resolves `source_run_id`'s workspace, finds the
Environment immediately above it by `promotion_rank` that has a
corresponding Workspace (matched by `name` within the Project - "the
`prod-vpc` workspace in staging promotes to the `prod-vpc` workspace in
production," not an arbitrary cross-workspace mapping the API would
otherwise have to ask the caller to specify every time), and creates a
new Run there with `trigger: promotion` and
`promoted_from_run_id` set - **the new Run still goes through the full
state machine from Stage 7 §1**, including that target environment's
own `requires_approval` policy check. Promotion is *not* a shortcut
around approval; it's specifically a way to guarantee "the exact
version that passed staging is what's promoted," not "trigger some
approximately-equivalent config in prod."

## 3. Variable resolution algorithm (backing §1's endpoint and the Run
## Dispatcher's internal use of it)

```
resolve(workspace_id, key):
  for scope in [workspace_id, workspace.environment_id, workspace.project_id, workspace.project.organization_id]:
    if scope is null: continue
    row = SELECT * FROM variables WHERE scope_id = scope AND key = key
    if row found: return row
  return not_found
```

Four-level cascade, checked in that exact order, first match wins -
directly implements Stage 3 §7, and is intentionally implemented as
**one shared internal library function**, called by both the
`GET .../variables` read endpoint (§1) and the Run Dispatcher (Stage 7
§3) before dispatching a Job - not reimplemented twice, since the two
call sites disagreeing about resolution order would be a very
hard-to-debug class of bug ("the UI shows one value, the actual apply
used a different one").

## 4. Where Secret resolution actually happens (tying Stage 3 §8's
## boundary to a concrete moment)

When the Run Dispatcher resolves a variable whose value is a
`SecretReference` rather than a plain value, **it does not fetch the
secret**. It passes the *reference* (mount_id + path) to the Worker as
part of the `JobAssignment` (Stage 4 §10), along with a short-lived
credential (requested by the Control Plane from the secret backend on
the worker's behalf, scoped to exactly that mount and expiring shortly
after the Job's timeout window) - **the Worker resolves the actual
value**, injects it into the running `terraform`/`ansible` process's
environment, and the value never transits back through the Control
Plane or gets written to the `jobs` table or any log line (the Worker's
log-forwarding path, Stage 7 §2's log stream, redacts any string
matching a resolved secret value before it ever leaves the Worker -
same posture as `compose-platform`'s own `log_scrubber` this operator
already built this session, generalized here to the platform level).

## 5. Background jobs

Deliberately none specific to this module beyond what Execution (Stage
7 §3) and Audit/Notifications (event consumers) already cover -
Workspace/Environment are primarily configuration aggregates, not
process-driving ones. The one thing worth naming explicitly: **there is
no separate "drift detection" background job in this module** - drift
detection is just a Scheduled Run Trigger (Stage 7 §3) configured with
`target: plan_only`, reusing the exact same mechanism as a scheduled
apply rather than being a distinct feature with its own code path.

## Open questions before the next module doc

1. **Workspace-name-matching promotion** (§2): confirmed same-name
   workspace across environments is how a promotion target resolves -
   is that the right default, or should promotion targets be an
   explicit mapping configured per-workspace instead (more setup, but
   removes the "what if names don't match" ambiguity entirely)?
2. **Which module next?** Policy and Approval are both still only
   referenced by interface (Stage 7 §1's state machine, this doc's
   Environment `requires_approval` flag) - recommend doing them
   together next since they're tightly coupled (a Policy check's result
   is what decides whether an Approval gets created at all, per Stage 7
   §1's transition table), unless you'd rather see GitOps next (closes
   the loop on how a Run actually gets triggered from a Git push in the
   first place).
