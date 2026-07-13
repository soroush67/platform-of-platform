# Module detail: Audit & Notifications

Eighth and final module-pair doc - completes all 16 Stage 3 contexts at
this level of detail. Both contexts had their *write* side fully
specified already (Stage 6: they're pure event-bus consumers, nothing
calls into them directly) - what's left is genuinely just their own
read/query/management API, which is why this doc is shorter than the
preceding seven.

## 1. REST API - Audit

```
GET /api/v1/orgs/{org}/audit-log
  ?actor_id=user:uuid
  &target_type=workspace&target_id=uuid
  &action=run.applied,run.canceled
  &occurred_after=2026-01-01T00:00:00Z
  &sort=-created_at        (cursor pagination, Stage 4 §3 - audit_entries
                             is explicitly one of the two tables Stage 5
                             §2 built cursor pagination for by name)
```

No `POST`/`PATCH`/`DELETE` - consistent with Stage 5's decision to
revoke UPDATE/DELETE grants on this table at the database-permission
level, this API layer doesn't even expose a code path that could try.

**One addition this doc makes**: `GET
/api/v1/orgs/{org}/audit-log/export` - streams the full filtered result
set as newline-delimited JSON (not paginated JSON, since the real use
case - feeding a SIEM, satisfying a compliance data request - wants the
whole matching set, not a UI-friendly page of it). Rate-limited more
aggressively than normal reads (Stage 4 §6's mechanism, a stricter
bucket specifically for this endpoint) since a full-org audit export is
a legitimately expensive query, not something that should be
retriable-in-a-loop by an API-key-holding script the way a normal list
call is.

## 2. REST API - Notifications

```
POST   /api/v1/orgs/{org}/notification-channels
  { "type": "slack", "scope": {"type": "project", "id": "..."},
    "config": {"webhook_url": "..."},
    "subscribed_event_types": ["run.completed", "run.failed", "approval.granted"] }
GET    /api/v1/orgs/{org}/notification-channels
PATCH  /api/v1/orgs/{org}/notification-channels/{channel}
DELETE /api/v1/orgs/{org}/notification-channels/{channel}
POST   /api/v1/orgs/{org}/notification-channels/{channel}/test
  → sends one synthetic test event through the real delivery path,
    same "verify before you trust it" pattern this doc keeps reusing
    (Stage 11 §1's secret-mount test-connection, compose-platform's own
    machine test-connection this session) - a channel misconfigured at
    creation time (wrong Slack webhook, unreachable SMTP host) should
    be caught by a deliberate test action, not discovered during the
    first real incident it was supposed to alert on.
```

`config`'s shape is `type`-dependent (webhook URL for slack/mattermost/
teams/webhook, SMTP fields for email) - stored as `jsonb` per Stage 5's
table map, validated server-side against a per-`type` schema at write
time rather than a rigid column set, the same reasoning as
`SecretMount.connection_config`'s shape in Stage 11 §1.

**Delivery failure handling**: per Stage 4 §8's outgoing-webhook retry
policy (exponential backoff, dead after 24h), extended here to *every*
channel type uniformly, not just the generic `webhook` one - a Slack
webhook or an SMTP send fails and retries exactly the same way a
third-party webhook does, because they're all the same underlying
"deliver this event to an external system that might be temporarily
down" problem. A channel that's failed every delivery attempt for 24h
gets a `status: degraded` flag surfaced in `GET
/notification-channels`, and (recursively, and deliberately not
infinitely) **that degradation itself fires a `notification.
channel_degraded` event through the same outbox/event pipeline** -
consumed by... Notifications itself, but specifically routed only to
channels *other than* the degraded one, and with a loop-breaker (a
channel's own degradation never re-triggers itself) - so an org
actually finds out "your Slack integration has been silently failing
for a day" through the same system, rather than the failure being
invisible until someone notices no alerts have arrived in a while.

## 3. Scope resolution for channels - same cascade shape as Variables,
## reused deliberately

A Notification Channel's `scope` (`organization | project | workspace`)
determines which events it receives: an event's own `organization_id`/
`project_id`/`workspace_id` (present on every domain event by
construction, since every aggregate resolves to the tenancy hierarchy
per Stage 3's closing cross-cutting note) is checked against every
Channel whose scope contains it - **cumulative, like Policy (Stage 9
§2), not first-match-wins like Variables** - an org-wide "alert
security-team on every failure" channel and a project-specific "alert
this team's Slack" channel should *both* fire for the same event, not
have the narrower one silently suppress the broader one. Explicitly
the opposite precedence direction from Variables and explicitly the
same direction as Policy, called out here so the pattern (some scoped
things cascade-and-override, some scoped things all apply) doesn't read
as inconsistent across the doc set - it's the same underlying
question ("is this a value with one right answer, or a set of things
that should all happen") asked three times, answered consistently by
that same question each time.

## Open questions before the Stage 0 process continues

1. **Audit export size limits**: §1's streaming export has no hard cap
   in this doc - confirm whether that's acceptable (a very large org
   with years of history could produce a genuinely large export) or
   whether it needs a bounded date-range requirement instead.
2. **All 16 Stage 3 contexts now have module-detail docs** (07 through
   14, covering Execution, Workspace/Environment, Policy/Approval,
   GitOps, Secrets/State, Kubernetes/Registry, Identity/RBAC/Tenancy,
   Audit/Notifications). Per Stage 0's original plan, the process moves
   to **UI → Backend → Workers → Integrations → Tests → Deployment**
   next. Confirm that's still the right next stage, or whether a
   cross-cutting doc capturing the patterns that recurred across
   multiple module docs (the "lightweight Worker job outside the Run
   state machine" shape used by Kubernetes Sync and Module Ingestion;
   the "cumulative vs first-match-wins" scope-resolution question asked
   three times) is worth writing first, so Backend-stage implementation
   has one place pointing at each pattern's one canonical explanation
   instead of three module docs each re-deriving it.
