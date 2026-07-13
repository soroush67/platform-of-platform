# Module detail: Policy & Approval

Third module doc, done together deliberately (as flagged at the end of
Stage 7's Workspace/Environment doc) because they're two halves of one
gate in the Run state machine (Stage 7 §1): Policy decides *whether this
change is even allowed*, Approval decides *whether a human still has to
say yes* for changes that are allowed but sensitive.

## 1. How this gate actually fires (closing the loop Stage 6 left as an interface)

```
Run reaches `planned`
        │
        ▼
Execution emits `run.plan_completed` (Stage 6)
        │
        ▼
Policy context consumes it: resolves every PolicySet in scope (§2),
fetches the plan output from object storage, evaluates each policy
inside a Worker Job (§4 - not the Control Plane), writes a
PolicyCheckResult, emits `policy.check_completed`
        │
        ▼
Execution consumes `policy.check_completed`:
  - hard violation present  → Run → `policy_failed` (terminal)
  - no violation, target Environment.requires_approval  → Run → `awaiting_approval`
  - no violation, no approval required                   → Run → `applying` (auto)
        │ (if awaiting_approval)
        ▼
Approval context creates an ApprovalRequest, snapshotting eligible
approvers (§5) at this moment
        │
        ▼
Human(s) submit decisions via §5's API
        │
        ▼
Approval emits `approval.granted` or `approval.rejected`
        │
        ▼
Execution consumes it: granted → `applying`; rejected → `canceled`
```

Every arrow here is an event, not a direct call between contexts - this
is the concrete worked example of Stage 6 §4's "each context reacting to
a fact about itself" rule, now traced through a full gate instead of
described abstractly.

## 2. Policy scope resolution - cumulative, not cascading (a real,
## deliberate difference from Variables)

Stage 3 §7's Variable resolution is *first-match-wins* (workspace beats
environment beats project beats org). **Policy is the opposite: every
matching PolicySet across every scope applies, cumulatively.** This is
deliberate, not an oversight - a Variable is "what value should this
key have," where exactly one answer makes sense; a Policy is "what's
disallowed," where an org-wide rule ("no public S3 buckets," "every
resource must be tagged") existing specifically so *no individual
workspace can opt out of it* is the entire point of having org-level
policy at all. If policy resolution were first-match-wins, a
workspace-scoped PolicySet would silently shadow the org's security
baseline - the one outcome an enterprise security team would consider
this feature broken for.

```
resolve_policy_sets(workspace_id):
  return PolicySets WHERE scope_id IN (
    workspace_id, workspace.environment_id,
    workspace.project_id, workspace.project.organization_id
  )
  -- all of them, not the first match
```

## 3. REST API - PolicySet

Paths relative to whichever scope it's created at:
`/api/v1/orgs/{org}/policy-sets` (org-scoped),
`.../projects/{project}/policy-sets`, or
`.../workspaces/{workspace}/policy-sets` - same resource shape at every
scope, per Stage 4's URL-mirrors-tenancy-hierarchy convention.

```json
// POST .../policy-sets
{
  "name": "no-public-buckets",
  "enforcement_level": "hard_mandatory",   // advisory | soft_mandatory | hard_mandatory, per Stage 3 §10
  "source": {
    "type": "git",                          // or "inline" for a quick org-authored check without a repo
    "git_connection_id": "uuid",
    "repo": "platform-policies", "path": "policies/s3.rego", "ref": "main"
  }
}
```

`type: "git"` is the expected path for anything an org actually
maintains long-term (policy-as-code reviewed via the same PR flow as
everything else, per the brief's "Everything as Code" principle) -
`inline` exists for the "I want to try one quick rule" case, stored
directly in the `policy_sets.policies` jsonb column from Stage 5,
without inventing a second storage mechanism for what's structurally
the same Rego source either way.

## 4. Where policy evaluation actually executes - inside a Worker, not the Control Plane

**This needed a real decision, not a default assumption.** Rego (OPA's
policy language) is generally treated as "safe" because it's
declarative and has no unbounded loops - but OPA ships real builtins
like `http.send` that can make outbound network calls from inside a
policy evaluation. An org's own security team wrote this Rego, so it's
more trusted than a random Terraform module from the public registry,
but Stage 2's control-plane/data-plane split exists specifically so the
Control Plane never executes code capable of reaching outside its own
trust boundary - and `http.send`-capable Rego qualifies. **Policy
evaluation runs as its own Job phase inside a Worker** (reusing the
exact same sandboxing/credential-scoping posture as any other Job,
Stage 2 §2), with OPA's Go SDK (`open-policy-agent/opa/rego` - embedded
as a library, not shelled out to a CLI, since Go is already this
platform's runtime per Stage 1 and OPA is itself a Go project with a
clean embeddable API) configured with a **restricted builtin set that
explicitly excludes `http.send` and any other I/O-capable builtin** -
policy evaluation gets the plan output as its only input, deterministic
and side-effect-free, by construction, not by trusting the policy
author's intentions.

## 5. REST API + resolution - Approval

### `GET /runs/{run}/approval`

```json
{
  "status": "pending",
  "required_approval_count": 2,
  "eligible_approvers": ["user:uuid1", "user:uuid2", "user:uuid3"],
  "decisions": [
    { "approver_id": "user:uuid1", "decision": "approve", "comment": "lgtm", "decided_at": "..." }
  ]
}
```

### `POST /runs/{run}/approval/decisions`

```json
{ "decision": "approve", "comment": "checked the plan, looks right" }
```

`403` if the caller isn't in `eligible_approvers` (the *snapshotted*
list from request-creation time, per Stage 3 §12 - a role granted
*after* the request was created doesn't retroactively make someone
eligible for this specific request, closing the same
time-of-check-vs-time-of-use gap Stage 3 flagged but this doc is the
first to spell out the enforcement rule for). `409` if already decided
by this approver, or if the request is no longer `pending`.

**Eligible-approver resolution** (at ApprovalRequest creation): every
User holding RBAC permission `workspace:approve` at the workspace's
own scope or any scope above it (environment/project/org - same
upward-implication rule as Stage 3 §4's RoleBinding evaluation).

**Self-approval**: `Environment.allow_self_approval` (new field this
doc adds to Stage 3's Environment aggregate - defaults `false` whenever
`requires_approval` is `true`). When `false`, the Run's own
`triggered_by` user is excluded from `eligible_approvers` even if their
role would otherwise qualify - the four-eyes control a real compliance
requirement (SOC2, most enterprise change-management policy) expects,
made a policy the platform enforces rather than a convention operators
have to remember to follow procedurally.

**`required_approval_count`**: also an `Environment` field (default 1),
not global - a low-stakes environment can stay single-approver while
production requires 2+.

## Open questions before the next module doc

1. **Policy evaluation failure handling**: if the OPA evaluation Job
   itself errors (bad Rego syntax, evaluation timeout) rather than
   *finding a violation* - should that block the Run (fail closed,
   treat an unevaluable policy as a hard violation) or let it proceed
   with a warning (fail open)? This doc recommends **fail closed** for
   `hard_mandatory`/`soft_mandatory` PolicySets and fail open only for
   `advisory` ones, but it's a real security-posture call worth your
   explicit sign-off rather than a default this doc silently picked.
2. **Which module next?** GitOps (closes the trigger side - how a Git
   push becomes the `run.plan_completed`-generating Run in the first
   place) is the natural next one since Execution, Workspace, and now
   Policy/Approval are all specified as consumers/reactors but nothing
   yet produces the *first* event in a real end-to-end flow.
