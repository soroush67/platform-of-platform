# Cross-cutting patterns

A catalog, not new design - every pattern here already exists in one of
docs 03-14, used at least twice, independently re-derived each time
because no earlier doc named it. This doc's only job is to give each one
a name and a decision rule, so **Backend/Workers/UI-stage
implementation, and any future module this platform grows, can cite
"Pattern: X" instead of re-deriving the same reasoning from scratch** -
and so a reviewer sees "this new feature uses the Snapshot Authorization
pattern" and immediately knows what that implies, rather than reading a
fresh paragraph of justification every time.

## Pattern: System Job (a Job with no Run)

**Used by**: Cluster Sync (Stage 12 §3), Module/Provider Version
Ingestion (Stage 12 §6).

**The shape**: dispatched through the exact same Worker gRPC mechanism
as a Run's Job (Stage 4 §10's `JobAssignment`/`ReportJobStatus`), but
**not owned by a Run, not subject to the Run state machine (Stage 7
§1), not policy-checked, not approval-gated** - because it doesn't
change managed infrastructure, it only *reads* something (a live
cluster's node list, a Git tag's content) to keep a Control-Plane-side
cache or catalog current.

**The rule for recognizing when a new feature needs this pattern rather
than a Run**: ask "does this action change infrastructure this platform
manages, or does it only read/sync something to keep our own metadata
current?" Changes → always a Run, no exceptions, even if it feels
lightweight (this is why Stage 12 §4 was explicit that cluster upgrades/
Cilium installs are Runs, not System Jobs, despite superficially
resembling a sync operation). Reads-only-to-refresh-a-cache → a System
Job.

**Storage**: a `system_jobs` table, deliberately separate from `jobs`
(Stage 5) - conflating the two would mean either polluting the Run-Job
schema with a nullable `run_id`, or a System Job accidentally becoming
visible in Run-scoped API responses that assume every Job belongs to a
Run.

## Pattern: Scope resolution - cumulative vs. first-match-wins

**Used by**: Variables (Stage 3 §7 / Stage 8 §3, first-match), Policy
(Stage 9 §2, cumulative), Notification Channels (Stage 14 §3,
cumulative).

**The decision rule, extracted and named so it doesn't need re-deriving
a fourth time**: for any concept scoped across the Tenancy hierarchy
(organization → project → environment/workspace), ask **"is this a
single VALUE that needs exactly one right answer, or an OBLIGATION/
SUBSCRIPTION where every applicable one should independently take
effect?"**

- **Value → first-match-wins**, narrowest scope overrides broader ones.
  A variable, a config default, anything where "which one wins" is the
  entire question.
- **Obligation/subscription → cumulative**, every matching scope
  applies simultaneously. A policy (an org-wide rule existing
  specifically so a narrower scope *can't* opt out), a notification
  subscription (an org-wide security alert and a project-specific Slack
  ping should both fire), an RBAC grant (Stage 3 §4's upward-implication
  - a broader-scope binding doesn't get silently overridden by a
  narrower one, it composes).

**Apply this test to any new scoped concept introduced during
Backend/UI implementation** before defaulting to whichever direction
feels more familiar from the last context worked on.

## Pattern: Test-before-trust

**Used by**: `SecretMount.../test-connection` (Stage 11 §1),
`NotificationChannel.../test` (Stage 14 §2) - and, outside this specific
platform, the exact same shape this operator already built and proved
this session in `compose-platform`'s `POST /machines/test-connection`.

**The rule**: any resource whose entire purpose is "hold credentials/
config to reach an external system" gets a `POST .../test` (or
`test-connection`) action that exercises the real integration path
(auth, a minimal real call) **without** committing to relying on it -
so a misconfiguration is caught at setup time by a deliberate act, not
discovered later when the thing it was supposed to do (alert on a
failure, resolve a secret) silently didn't happen.

## Pattern: Shown-once secrets

**Used by**: API key plaintext (Stage 13 §2) - and, outside this
platform, `vault-ha`'s own `init.yml` Shamir unseal-key output this
operator built earlier this session.

**The rule**: a value the platform itself generates (not one an
operator provides) and that grants real access, displayed exactly once
at creation in the API response / Web UI, **never stored in a
retrievable form and never returned by any later `GET`** - only a
non-reversible identifier (a hash, a last-4-characters display) persists
server-side. Losing it means reissuing, not "asking support to look it
up," by construction.

## Pattern: Presigned redirect for large/blob content

**Used by**: state version downloads (Stage 11 §3), Run plan/apply
output and logs (Stage 7 §2), module/provider packages (implied by
Stage 12 §6's object storage layout).

**The rule**: any response body that's a large, mostly-opaque blob
(more than a few KB, generated by a Worker rather than typed by a user)
is **never proxied through the Control Plane** - the API returns a
`307` redirect to a short-TTL presigned object storage URL instead.
Keeps the Control Plane's own resource profile (memory, connection
duration) decoupled from artifact size, and keeps the
"Control-Plane-never-handles-raw-third-party-execution-output" posture
from Stage 2 consistent even for content that isn't itself executable.

## Pattern: Cache with an explicit staleness marker

**Used by**: Cluster node inventory / `health_status` (Stage 12 §1,
"never mistake this for live state"), the Web UI's CQRS read models
(Stage 6 §5).

**The rule**: any denormalized or synced-from-elsewhere read model is
**always returned with the timestamp of its own last update**
(`synced_at`, `as_of`, or equivalent) alongside the data - so no API
consumer or UI can conflate "what this endpoint returns" with "the true
current state," and staleness is a visible property of the response,
not a fact only the backend knows.

## Pattern: Soft-delete with scheduled purge

**Used by**: Organizations (Stage 13 §1) - and, outside this platform,
`compose-platform`'s existing Users/Machines-with-audit-history
handling this operator already built this session.

**The rule**: anything with FK-referenced audit/history data that would
be orphaned by a hard delete gets `status: archived` (hidden from
active listings, functionally inert) plus a scheduled purge job at a
configurable grace period, **not an immediate `DELETE`** - Postgres CIS
integrity and Audit's own append-only guarantee (Stage 5 §2) both stay
intact through the grace period, and the purge, when it does run, is a
deliberate scheduled event with a paper trail, not a synchronous side
effect of one API call.

## Pattern: Snapshot, not live query, for time-sensitive authorization

**Used by**: `ApprovalRequest.eligible_approvers` (Stage 3 §12, Stage 9
§5).

**The rule**: when "who's allowed to do X" is evaluated once to gate a
specific, time-bound decision (as opposed to "who's allowed to do X"
evaluated fresh on every request, which is RBAC's normal mode
everywhere else), **the eligible set is captured at the moment the
decision-point is created, not re-resolved at decision time** - a role
change after the fact can't retroactively add or remove someone's
standing to approve a request that already existed, closing the
specific time-of-check-vs-time-of-use gap this pattern exists to name.
Rare, deliberate exception to "RBAC is always evaluated live" - flagged
as its own named pattern specifically so a future module doesn't apply
it by accident where live evaluation was actually correct.

## What this doc is NOT

Not a 17th bounded context, not new scope, not an API surface of its
own - purely a naming/indexing pass over decisions already made and
already committed in docs 03-14. Nothing in this doc should ever be
cited as the *origin* of a decision (each pattern's origin doc is
listed under "Used by") - only as the *rule* to apply the next time a
similar situation comes up.

## Next: Stage 0's planned sequence resumes - UI

With all 16 contexts detailed and their recurring patterns named, the
process moves into UI design per the original plan (Stage 0 §"How this
doc set is organized").
