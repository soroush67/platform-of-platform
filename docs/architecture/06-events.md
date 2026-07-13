# Events

## 1. This is event *notification*, not event *sourcing* - said explicitly because the spec's wording could be read either way

The brief lists "Event Driven Architecture" and "CQRS where useful."
Worth being precise about what that does and doesn't mean here, because
the two real interpretations lead to very different systems:

- **Event sourcing** (the events *are* the system of record; current
  state is rebuilt by replaying them) - **not what this is**. It buys
  a genuinely powerful audit/replay story at the cost of every aggregate
  needing snapshotting, every event schema needing forever-compatible
  evolution, and every read needing either a replay or a maintained
  projection. That cost doesn't pay for itself here: Stage 5 already
  committed Postgres as the system of record specifically because this
  domain's aggregates (a Workspace, a Run) have simple, well-understood
  current-state semantics that don't need to be *derived* - they need to
  be *queried*, fast, with RLS enforcing isolation on the query itself.
- **Event notification / event-carried state transfer** (chosen): a
  domain event is a *fact that already happened* to state that already
  lives durably in Postgres (Stage 5). Consumers (Audit, Notifications,
  the Web UI live feed, future third-party webhooks) react to facts;
  none of them are how the *authoritative* copy of a Run's status gets
  determined. This is the same distinction Stage 3 drew explicitly for
  Audit: "populated exclusively by subscribing to events, no context
  calls into it directly" is a notification consumer, not an
  event-sourced projection standing in for a real table.

## 2. The correctness problem this design has to solve: the dual-write

A domain event is only trustworthy if "the Postgres transaction
committed" and "the event was published to NATS" are atomic together -
neither can happen without the other. Publishing to NATS *inside* the
same database transaction is impossible (they're different systems);
publishing *after* commit in application code is a real, common bug
class (the process crashes between the commit and the publish call, and
the event is silently lost forever - Audit and Notifications never learn
a Run completed, with no error anywhere).

**Solution: the Transactional Outbox pattern.**

```sql
create table outbox_events (
  id           uuid primary key default gen_random_uuid(),
  organization_id uuid not null,          -- RLS applies here too
  event_type   text not null,              -- 'run.completed', 'workspace.created', ...
  payload      jsonb not null,
  occurred_at  timestamptz not null default now(),
  published_at timestamptz                 -- null until the relay confirms NATS ack
);
```

Every domain operation that emits an event **writes to `outbox_events`
in the exact same transaction as the state change it's reporting** - a
`Run` transitioning to `applied` and the `run.completed` outbox row are
one atomic commit, or neither happens. A separate, small **Outbox Relay**
process (part of the Control Plane, not its own deployable - it's a
background goroutine, not a new operational unit) polls
`outbox_events WHERE published_at IS NULL ORDER BY occurred_at`
(or, as a later optimization once polling latency actually matters,
switches to Postgres logical replication / `pg_notify` to wake
immediately instead of polling), publishes to the matching NATS
JetStream subject, and marks `published_at` on ack. **At-least-once
delivery, by construction** - a relay crash before marking `published_at`
just means the same event gets republished on restart, which is exactly
why every consumer (§4) must be idempotent, not an edge case to handle
someday.

This is the single most important mechanism in this document - every
other event-related design decision assumes it's correct.

## 3. Event envelope

One shape for every event on the bus, matching the outgoing-webhook
envelope from Stage 4 §8 deliberately (a webhook subscriber is just
another consumer of the same fact stream, so it gets the same shape,
not a translated one):

```json
{
  "event_id": "uuid",
  "event_type": "run.completed",
  "event_version": 1,
  "organization_id": "uuid",
  "occurred_at": "2026-07-14T10:00:00Z",
  "data": { "...": "event-type-specific payload" }
}
```

`event_version` is per-`event_type`, incremented only on a
backward-incompatible payload change (a new optional field is not a
version bump; removing or retyping a field is). Old consumers that
haven't been updated for a new version keep receiving the version they
understand - the relay publishes to a version-suffixed subject
(`run.completed.v1`) precisely so this is a routing decision, not a
runtime branch in every consumer.

## 4. NATS subject naming + consumer map

Subject hierarchy: `platform.{context}.{aggregate}.{action}.v{n}` -
chosen so wildcard subscriptions align exactly with how consumers
actually want to subscribe (Audit wants *everything*: `platform.>`;
Notifications wants *per-org-configured event types*:
`platform.execution.run.completed.v1`, etc.):

| Subject pattern | Emitted by (Stage 3 context) | Consumed by |
|---|---|---|
| `platform.tenancy.organization.*.v1` | Tenancy | Audit |
| `platform.identity.user.*.v1` | Identity & Access | Audit |
| `platform.workspace.*.v1` | Workspace & Environment | Audit, Notifications |
| `platform.execution.run.*.v1` | Execution | Audit, Notifications, Web UI live feed (bridged via a WebSocket gateway that itself is just another JetStream consumer), Approval Workflow (listens for `run.plan_completed` to decide if it needs to create an ApprovalRequest) |
| `platform.gitops.webhook_received.v1` | GitOps | Execution (interprets it into a new Run per Stage 3 §11 - this is the one case where a context's event directly causes another context's command, everywhere else consumption is purely reactive/notify-only) |
| `platform.policy.check_completed.v1` | Policy | Execution (gates the Run's state machine), Audit, Notifications |
| `platform.approval.*.v1` | Approval Workflow | Execution (unblocks the Run), Audit, Notifications |
| every other context | (same pattern) | Audit always; Notifications if an org subscribed to that type |

**The one exception to "events are pure notification" worth calling out
explicitly**: `gitops.webhook_received` → Execution creating a Run, and
`policy.check_completed`/`approval.*` → Execution's own state machine
reacting. These are still each *"my own context reacting to a fact,"*
not one context reaching into another's data - Execution's Run state
machine subscribing to Policy's and Approval's events is exactly the
same shape as Notifications subscribing to Execution's events, just
with "the reaction is a state transition" instead of "the reaction is a
Slack message." No context ever calls another context's internal API to
make something happen; it only ever reacts to what already happened.

## 5. CQRS - specifically where it earns its cost, not everywhere

Applied in exactly two places, each independently justified rather than
adopted as a blanket pattern:

- **Web UI live event timeline / dashboard**: a denormalized read model
  (a `run_timeline_view` table or a Redis-backed projection, TBD at
  Backend stage) built by a consumer of the same event stream every
  other consumer reads, kept eventually consistent with the Postgres
  writes it summarizes. Justified because the dashboard's actual query
  shape ("show me the last 50 events across every workspace in this
  org, joined with workspace/run names") is expensive to serve from
  normalized tables under real load, and it's read-heavy,
  latency-sensitive UI - exactly CQRS's home turf.
- **Search** (Stage 1 explicitly deferred a dedicated search engine, but
  when it lands): a search index is *definitionally* a CQRS read model -
  built from the same event stream, never the source of truth, always
  safe to rebuild from scratch by replaying the outbox/event history.

Everywhere else (a Workspace's own detail page, an Organization's
member list, ...) is a **normal synchronous read from the Postgres
tables in Stage 5**, on purpose - introducing a read-model/projection
for every simple CRUD-shaped read would mean maintaining eventual
consistency for views that gain nothing from it.

## Open questions before Stage 7 (per-module detail)

1. **Outbox relay failure mode**: confirmed at-least-once, meaning
   double-delivery is possible (relay crash after NATS ack, before
   marking `published_at`) - every consumer in §4 must dedupe by
   `event_id`. Confirming this is an acceptable operational contract
   (it's the standard one for this pattern) rather than something that
   needs stronger guarantees (which would mean NATS JetStream exactly-
   once semantics + more complex relay bookkeeping, for a correctness
   property none of today's consumers - Audit, Notifications, dashboards
   - actually need, since all three are naturally idempotent-safe:
   re-inserting an already-seen audit entry ID or re-sending an
   already-sent notification with the same idempotency key is harmless).
2. Ready to move into **per-module detail** (Stage 7) - this is where
   the process gets genuinely large (each of the 16 Stage 3 contexts
   gets its own full REST endpoint list, request/response shapes tied to
   real DB columns, and its own background jobs). Confirm you want that
   done context-by-context as separate follow-up docs (so each is
   independently reviewable) rather than one very large combined doc.
