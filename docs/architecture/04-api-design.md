# API Design

Per Stage 1's challenge to the original spec: **REST is the one stable,
versioned, public contract.** This doc designs that contract's shape and
conventions. gRPC (internal Control-Plane-to-Worker only) and the future
GraphQL federated gateway are covered briefly at the end - they're not
where the design effort goes at this stage.

## 1. Resource model - URL shape mirrors the Stage 3 context map

Every REST resource path is rooted at the Tenancy hierarchy from Stage 3,
because every aggregate resolves to an Organization and RBAC scope
checks need that resolvable from the URL itself, not looked up
separately:

```
/api/v1/orgs/{org}
/api/v1/orgs/{org}/projects/{project}
/api/v1/orgs/{org}/projects/{project}/environments/{environment}
/api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}
/api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}/runs/{run}
/api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}/runs/{run}/jobs/{job}
/api/v1/orgs/{org}/teams/{team}
/api/v1/orgs/{org}/service-accounts/{sa}
/api/v1/orgs/{org}/git-connections/{connection}
/api/v1/orgs/{org}/secret-mounts/{mount}
/api/v1/orgs/{org}/policy-sets/{policy_set}
/api/v1/orgs/{org}/clusters/{cluster}
/api/v1/orgs/{org}/modules/{namespace}/{name}
/api/v1/orgs/{org}/notification-channels/{channel}
/api/v1/orgs/{org}/audit-log
```

`{org}`/`{project}`/etc. are **slugs, not internal UUIDs**, in the URL -
UUIDs stay as the actual primary key/foreign key in the database and in
response bodies (`id` field), but the URL a human or a `terraform{...}`
CI config hardcodes should be a stable, readable slug, not an opaque ID
that's annoying to reference in version-controlled config. This is the
same reasoning GitHub/GitLab repo URLs use owner/repo slugs, not numeric
IDs.

**Approval, Policy check results, and State versions are not their own
top-level collections** - they're always accessed as sub-resources of
the Run that produced them (`/runs/{run}/approval`,
`/runs/{run}/policy-checks`, `/workspaces/{workspace}/state-versions`),
matching that they have no independent identity/lifecycle outside their
parent aggregate (Stage 3's entity-vs-aggregate calls).

## 2. Versioning

**URL path versioning** (`/api/v1/...`), not header-based content
negotiation. Reasoning: this API's actual clients (the CLI, Terraform's
own remote-backend HTTP protocol conventions, third-party CI scripts
using plain `curl`) are exactly the audience URL versioning serves best
- it's inspectable in a browser, greppable in scripts, and doesn't
require every client library to correctly implement `Accept` header
negotiation to get a determinate response. A `v1` → `v2` bump is a new
URL prefix, both served simultaneously during a deprecation window - no
in-place breaking changes to `v1` ever, per the Twelve-Factor/backward-
compatibility principles the brief asked for.

## 3. Pagination, filtering, sorting

Cursor-based pagination (not offset/limit) for every list endpoint,
because the two things this platform's collections are most often listed
by - **Runs** and **Audit entries** - are both high-write-volume,
append-mostly streams where offset pagination silently skips or
duplicates rows as new items are inserted between page fetches. One
convention for every collection, not "cursor for Runs, offset for
everything else":

```
GET /api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}/runs
  ?limit=50
  &cursor=<opaque_token>
  &status=applied,failed          (filter, comma = OR)
  &created_after=2026-01-01T00:00:00Z
  &sort=-created_at                (- prefix = descending; sort field allowlisted per endpoint)

Response:
{
  "data": [ ... ],
  "next_cursor": "<opaque_token>" | null
}
```

## 4. Authentication

Three credential types, all resolving to the same internal
`Principal{subject, organization_id, permissions}` the RBAC context
(Stage 3 §4) evaluates against - the API layer never branches its
authorization logic by credential type, only its *authentication* logic
does:

- **User session** (Web UI): OIDC/SAML/LDAP/local login → short-lived
  JWT access token + refresh token, standard practice, nothing novel
  here.
- **API Key** (CLI, scripts, CI): `Authorization: Bearer <key>`,
  resolved via the Identity context's APIKey entity (Stage 3 §3) -
  long-lived by design (CI systems can't do an interactive OIDC dance),
  which is exactly why APIKey has its own `scopes` narrowing field
  distinct from the owner's full RBAC grant: a CI-issued key should be
  mintable with only `workspace:plan` and `workspace:apply` on one
  specific workspace, not the full grant of the human who created it.
- **Worker-to-Control-Plane** (internal, not on the public API at all):
  mTLS + a short-lived worker identity token issued at worker
  registration - covered fully in the Workers/Execution stage, not
  here, since it never touches the public REST surface.

## 5. Idempotency

**Every state-mutating endpoint that a CI script or webhook handler
might legitimately retry accepts an `Idempotency-Key` header** (the
`POST /runs` "trigger a run" endpoint is the concrete case that matters
most: a flaky network causing a CI job to retry a run-trigger call must
never queue two runs). Server behavior: first request with a given key
executes and caches `(key → response)` for 24h; a repeated request with
the same key inside that window returns the cached response without
re-executing. Standard pattern (Stripe popularized it for exactly this
class of problem), applied here specifically because "did my apply run
twice" is a much worse failure mode for this product than for a typical
CRUD API.

## 6. Rate limiting

Per-Principal (not per-IP - a CI system and ten developers can share an
egress IP) token bucket, limits configurable per Organization (a paid
tier concern for later, but the mechanism is generic from day one).
`429` responses carry `Retry-After` and `X-RateLimit-{Limit,Remaining,
Reset}` headers - standard, and specifically what makes the CLI able to
back off automatically rather than hot-looping a customer's CI runner
against their own quota.

## 7. Errors

RFC 7807 `application/problem+json` for every error response - one
consistent shape (`type, title, status, detail, instance`) plus a
platform-specific `errors[]` array for field-level validation failures.
Chosen over a bespoke error envelope because it's an actual IETF
standard several client-generation tools already understand, reducing
custom error-handling code in the generated SDKs (§9).

## 8. Webhooks

**Incoming** (Git providers → this platform, feeding the GitOps
context's `WebhookReceived` event from Stage 3 §11): one endpoint per
provider (`/api/v1/webhooks/github`, `/webhooks/gitlab`, ...) since each
provider's payload shape and signature-verification scheme differs -
trying to force one generic endpoint here would just move the
provider-specific parsing into a query param instead of the URL, no
real simplification.

**Outgoing** (this platform → third parties, the generic case behind
the Notification context's `webhook` channel type from Stage 3 §16):
HMAC-SHA256 signed payload (`X-Signature` header, shared secret set at
channel creation), a fixed envelope `{event_type, occurred_at, org_id,
data}`, and the platform's own retry policy (exponential backoff, dead
after 24h of failures, surfaced back to the org as a degraded-channel
warning rather than failing silently).

## 9. OpenAPI + SDK generation

The REST API is **defined API-first**: an OpenAPI 3.1 spec is the source
of truth (hand-written/reviewed, not reverse-generated from server code
after the fact - reverse-generation tends to leak implementation
accidents like a Go struct field name into the public contract). Server
route handlers and client SDKs (Go, Python, TypeScript at minimum,
matching the languages already in play across this operator's other
projects and this platform's own Go/TS stack) are both generated from
that one spec, so the spec can never drift silently out of sync with
either side.

## 10. Internal gRPC protocol (Control Plane ↔ Workers) - shape only

Not the public API, so specified only to the depth needed to confirm
Stage 2's design holds together; full `.proto` definitions belong to the
later Workers stage.

```protobuf
service WorkerDispatch {
  rpc StreamJobs(WorkerRegistration) returns (stream JobAssignment);
  rpc ReportJobStatus(JobStatusUpdate) returns (Ack);
  rpc StreamJobLogs(stream LogChunk) returns (Ack);
}
```

A Worker opens one long-lived `StreamJobs` connection at startup
(registering its pool/engine-type capabilities), receives `JobAssignment`
messages as work becomes available (this *is* the NATS JetStream
consumer from Stage 2, bridged into the gRPC stream server-side - Workers
never talk to NATS directly, keeping the message-bus credential entirely
inside the Control Plane's trust boundary), and pushes status/log updates
back over the same connection. One bidirectional stream per worker, not
a request per job - deliberately, to avoid a worker needing a routable
inbound address (Workers can sit behind NAT/in a private subnet with no
inbound firewall rule needed, only outbound to the Control Plane - a real
requirement for the "Remote Workers" item in the original spec).

## 11. GraphQL (deferred, per Stage 1) - where it will sit when built

A single federated gateway process, reading the same REST resources (or,
more likely by then, reading directly from the same Go domain
services the REST handlers call, to avoid a REST-calling-REST hop) -
purely additive, never a second way to *write* data (all mutations stay
REST-first) to avoid two parallel and potentially-diverging write paths
into the same aggregates.

## Open questions before Stage 5 (database)

1. **Slug immutability**: are Org/Project/Workspace slugs allowed to be
   renamed after creation? If yes, every hardcoded URL a customer's CI
   config references breaks on rename - the common resolution (Terraform
   Cloud, GitHub) is "slugs can change, the old slug 404s immediately,
   no redirect" - confirm that's acceptable, or require immutable slugs
   (simpler, less flexible) instead.
2. **API Key scopes granularity**: Stage 3 mentioned scopes "narrowing
   below the owner's own RBAC grant" but didn't fix the granularity -
   confirm whether scopes should mirror RBAC's own Permission value
   objects exactly (`workspace:apply`) or be coarser
   (`read-only` / `plan-only` / `full-access` presets). Coarser is
   faster to ship; fine-grained is what a security-conscious enterprise
   buyer will actually ask for.

**Resolved (operator approved proceeding with these defaults, both
easy to revisit before any code exists)**:

1. **Slugs are renamable, old slug 404s immediately, no redirect** -
   matches GitHub/GitLab/Terraform Cloud; avoids building and
   maintaining an alias-resolution table for a case (rename) that's rare
   relative to normal reads.
2. **Fine-grained, RBAC-Permission-level API key scopes** - the
   Permission enum from Stage 3 §4 already exists for RBAC, so scopes
   are literally "a subset of that same enum," not new modeling work.
   Coarse presets (`read-only`, `plan-only`) get layered on top later as
   a *Web UI convenience* (a preset button that just pre-selects a
   canned permission set) - choosing fine-grained now doesn't block
   adding that convenience later, whereas shipping only coarse presets
   now and needing fine-grained later would mean redesigning the APIKey
   scope storage shape under existing customer keys.
