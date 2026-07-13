# Module detail: Execution (Run, Job)

First of the per-module docs (Stage 7 onward, one context at a time per
the plan agreed at the end of Stage 6). Execution goes first because
every other context either feeds it (GitOps, Policy, Approval) or
depends on its state (Web UI, Notifications, Audit) - getting its state
machine and API precise first makes every later module doc faster to
write against something concrete instead of another abstraction.

## 1. Run state machine - the actual transition table

Stage 3 listed the status set; this is where "which actions are legal
from which status" gets nailed down, because it's referenced by name
from Policy and Approval's own event handling (Stage 6 §4):

```
queued ──dispatch──▶ planning ──plan success──▶ planned
                          │                         │
                     plan failure                   │
                          │                    policy check
                          ▼                          │
                       failed              ┌──────────┴──────────┐
                                     no policy violation    hard violation
                                            │                    │
                                            ▼                    ▼
                              (requires_approval?)          policy_failed
                                    │        │
                                   yes       no
                                    │        │
                                    ▼        │
                          awaiting_approval  │
                                    │        │
                              approved/      │
                              (auto if       │
                               not required) │
                                    └────┬───┘
                                         ▼
                                    applying ──success──▶ applied
                                         │
                                    apply failure
                                         │
                                         ▼
                                       failed

  any of {queued, planning, awaiting_approval, applying}
        ──user/API cancel──▶ canceled

  any non-terminal status, no progress for > worker_timeout
        ──Stale Run Reaper (§5)──▶ errored
```

**Terminal statuses** (release the workspace lock, Stage 5's
`SELECT...FOR UPDATE` pattern): `applied, failed, policy_failed,
canceled, errored`. Every other status holds the lock. This table *is*
the authoritative spec for what `POST /runs/{run}/cancel` and
`POST /runs/{run}/apply` are allowed to do (§2) - both endpoints reject
with `409 Conflict` (not `400` - the request is well-formed, the
*current state* just doesn't allow it) outside their listed source
statuses.

## 2. REST API

All paths relative to
`/api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}`, per
Stage 4's resource model.

### `POST /runs` - trigger a run

Requires `Idempotency-Key` (Stage 4 §5 - this is *the* endpoint that
motivated that requirement).

```json
// Request
{
  "trigger": "manual",                    // manual | api - vcs_push/vcs_pr/scheduled are set internally, never client-supplied
  "target": "plan_and_apply",             // plan_only | plan_and_apply
  "variable_overrides": { "key": "value" } // optional, this run only - doesn't persist to the Variable context
}
// 202 Accepted
{
  "id": "uuid", "status": "queued", "workspace_id": "...",
  "trigger": "manual", "triggered_by": "user:uuid",
  "created_at": "..."
}
```

**Why `202` not `201`**: a Run isn't "created" in the REST-resource
sense at the moment this returns - it's accepted into a queue whose
actual execution start time is indeterminate (depends on worker
availability). `202` is the correct HTTP semantic for "accepted for
async processing," and it's what tells a well-behaved client to poll
`GET /runs/{run}` or open the log stream rather than assume the run
already started.

### `GET /runs` - list (cursor-paginated, per Stage 4 §3)

`?status=applied,failed&created_after=...&triggered_by=user:uuid&sort=-created_at`

### `GET /runs/{run}`

Full Run resource: every column from Stage 5's `runs` table, plus
`jobs: [{id, phase, status}]` summary (not full job detail - that's
`GET /runs/{run}/jobs/{job}`) and, if a policy check or approval is
attached, their current status inlined (`policy_check: {status,
violations_count}`, `approval: {status, approvals_received,
approvals_required}`) - denormalized into the response specifically
because "is this run waiting on me" is the single most common thing the
Web UI asks, and forcing three separate requests (run, policy, approval)
to answer it would be the wrong tradeoff.

### `POST /runs/{run}/apply` - proceed from `planned`/`awaiting_approval` to `applying`

Empty body. `409` if the Run isn't in an apply-eligible status per §1's
table. Also requires `Idempotency-Key` (same reasoning as `POST /runs`).

### `POST /runs/{run}/cancel`

Empty body, `202` (cancellation is itself async - it asks the assigned
Worker to terminate the running process, which isn't instantaneous;
the Run's status becomes `canceled` only once the Worker confirms via
the gRPC status stream, Stage 4 §10).

### `GET /runs/{run}/plan-output` / `GET /runs/{run}/apply-output`

`307` redirect to a presigned, short-TTL object storage URL - the
Control Plane never proxies large blob bodies through itself.

### `GET /runs/{run}/jobs`, `GET /runs/{run}/jobs/{job}`

Job resources per Stage 5's `jobs` table.

### `GET /runs/{run}/jobs/{job}/logs`

Same presigned-redirect pattern as plan-output, for the completed,
durable log. For a **still-running** job, redirects instead to a
`wss://.../runs/{run}/jobs/{job}/logs/stream` WebSocket endpoint that
bridges to the Redis pub/sub channel from Stage 5 §5 - the client asks
for "the logs" once and the API decides whether that means "read the
finished file" or "subscribe to the live stream" based on the job's
actual status, rather than the client having to know which to ask for.

## 3. Background jobs (the operational half of this module - not
## request/response, but what makes Runs actually progress)

- **Run Dispatcher**: a Control Plane background loop consuming
  `queued` Runs (via a `SELECT ... FOR UPDATE SKIP LOCKED` claim query,
  not a NATS subject - dispatching is an internal scheduling decision
  over Postgres-resident state, not a domain event anyone else needs to
  react to) and, for each, resolving an available Worker whose
  registered capabilities (Stage 4 §10's `WorkerRegistration`) match the
  Workspace's `execution_engine` and any configured worker-affinity
  labels, then sending the `JobAssignment` over that Worker's open gRPC
  stream.
- **Stale Run Reaper**: periodic (every 30s) sweep for Runs in a
  non-terminal status whose assigned Job hasn't reported a status update
  via gRPC within a timeout window (default 10 minutes, configurable per
  Workspace for genuinely long-running engines like a multi-hour
  Kubespray run) - transitions them to `errored`, releases the workspace
  lock, and emits `run.errored` onto the outbox (Stage 6). **This is the
  single most commonly-missed piece in a first-pass execution-engine
  design**: without it, a Worker crash (OOM-killed mid-`terraform
  apply`, the pod evicted) leaves the Workspace permanently locked with
  no automatic recovery - worth stating explicitly as a designed-for
  failure mode, not an edge case discovered in production.
- **Scheduled Run Trigger**: a cron-style evaluator (checks a
  `scheduled_runs` config per Workspace - "plan this workspace nightly
  for drift detection," "auto-apply this workspace every Monday 9am") -
  produces the same `POST /runs`-equivalent internal command as a manual
  trigger, just with `trigger: scheduled` and `triggered_by: "system"`.
- **Run Triggers** (cross-workspace dependency, the spec's
  "Dependencies" requirement): a Workspace can declare
  `run_trigger_source_workspace_ids` - when a source workspace's Run
  reaches `applied`, the Run Dispatcher (or a small dedicated consumer of
  `run.applied` events) creates a new queued Run on every dependent
  workspace. Modeled as *workspace configuration consumed by a Run
  Dispatcher event-subscription*, not a new aggregate - it doesn't need
  its own identity/lifecycle beyond "workspace A's completion triggers
  workspace B."

## 4. What's explicitly out of scope for this doc

- The Worker-side implementation of actually invoking
  `terraform`/`ansible-playbook`/`helm` (that's the Workers stage, a
  peer to this doc, not a subsection of it - Execution owns the Run/Job
  *lifecycle and API*, Workers own *how a Job's phase actually gets
  executed*).
- Policy check and Approval request creation logic in detail - both are
  their own module docs; this doc only fixes the *interface* (which Run
  statuses exist because of them, which events Execution reacts to).

## Open questions before the next module doc

1. Confirm the state machine in §1, specifically: should
   `policy_failed` be a distinct terminal status from `failed` (this
   doc's choice - lets the Web UI and Notifications distinguish "your
   infrastructure code has a bug" from "your infrastructure code is
   fine but violates an org policy," which are different audiences to
   notify and different remediation actions), or should policy
   violations just be a `failed` Run with a reason code instead?
2. **Which module doc next?** Given Execution's own dependencies just
   surfaced concretely (Policy and Approval both gate its state
   machine, GitOps feeds it, Workers execute it) - Workspace &
   Environment (the aggregate Runs belong to, and where Variables/
   Secrets resolution actually happens before a Job starts) is the next
   natural one, or Policy/Approval since this doc just leaned on their
   interfaces without defining them. Your call.
