# Architecture style, runtime choice, and challenges to the original spec

This doc does the two things the process asked for before any domain
modeling starts: pick the foundational, hard-to-reverse decisions, and
push back explicitly on parts of the spec that would hurt the product if
built exactly as listed.

## 1. Modular Monolith first, not microservices-by-default

The spec itself says "Microservices only when justified, Otherwise Modular
Monolith" - this doc takes that seriously rather than treating the long
module list as an implicit map of 40 microservices.

**Decision: one deployable core platform binary/process group (the
"Control Plane"), organized internally as strict bounded contexts with
enforced module boundaries (hexagonal ports/adapters, no module reaching
into another's database tables) - plus a small number of genuinely
separate deployables where the isolation reason is concrete, not
speculative:**

| Deployable | Why it's separate, not a module inside the monolith |
|---|---|
| **Control Plane** (Identity, Orgs/Projects/Teams, RBAC, Workspaces, Environments, Registry, Secrets *metadata*, Variables, Policy, Notifications, Approvals, Audit, GitOps orchestration, public API) | Default home for everything. Splitting these prematurely means N services doing chatty synchronous calls to each other for what's really one consistent transaction boundary (e.g. "create workspace" touches RBAC + audit + notifications in one unit of work). |
| **Execution Workers** (Terraform/OpenTofu/Ansible/Helm/Compose/Packer/Kubespray runners) | Genuinely different resource profile (CPU/memory spikes, long-running, sometimes untrusted third-party module/provider code), genuinely different scaling axis (scale with concurrent runs, not with API request volume), and a real security boundary (a `terraform apply` running arbitrary provider code should not share a process/trust domain with the API that issues tokens). This is the same reasoning Atlantis, Spacelift, and TFC all landed on independently. |
| **Event/Notification Dispatcher** | Fan-out to Slack/Mattermost/Teams/email/webhooks is I/O-bound, retries independently of the thing that triggered the event, and must not block the Control Plane transaction that emitted the event. Small, stateless, horizontally scalable on its own. |

Everything else in the spec's module list (Kubernetes Engine, Vault
Integration, Git Integration, Module/Provider Registry, State Management,
...) is a **bounded context inside the Control Plane**, not its own
deployable, until real load data says otherwise (Strangler Fig: pull a
module out only once its actual traffic/scaling/team-ownership profile
justifies the operational cost of a network boundary). Justifying this
concretely, not just "microservices are more scalable" - a network hop
you didn't need is strictly worse than a function call: it adds latency,
a new failure mode, and a new thing to version together.

## 2. Runtime/language: Go for Control Plane + Workers, TypeScript/React for the Web UI

Not specified in the original spec - this is a foundational choice this
doc is making explicitly rather than leaving implicit, because it gates
almost every later decision (concurrency model, plugin ABI, distribution
story for air-gap).

**Recommendation: Go**, for both the Control Plane and the Execution
Workers. Reasoning:

- **The exact prior art this product overlaps with is Go**: Terraform,
  OpenTofu, Vault, Nomad, Boundary, Consul, Argo CD, Crossplane, k3s/RKE2
  are all Go. That's not a coincidence - Go's stdlib concurrency model
  fits "orchestrate many long-running external processes and stream
  their output" (exactly what every execution engine needs) unusually
  well, and its `client-go`/controller-runtime ecosystem is the de facto
  standard for anything touching the Kubernetes API.
- **Single static binary distribution is a direct, load-bearing win for
  the air-gap requirement.** No runtime to stage offline, no interpreter
  version drift between the machine that built a plugin and the machine
  running it. This is the same reason this operator's own kubespray-webui
  and vault-ha projects lean on real-container-based offline staging
  rather than assuming a package manager is reachable - Go's distribution
  model removes an entire class of offline-packaging problems other
  runtimes (Python, Node, JVM) don't.
- **Plugin architecture**: Go's `plugin` package is genuinely painful
  (version-locked, platform-locked); the practical answer - used by
  Terraform, Vault, and Packer's own plugin systems - is **RPC-based
  plugins over a local Unix socket or gRPC**, i.e. a plugin is its own
  subprocess speaking a defined protocol. This composes cleanly with the
  "Execution Engines must support plugins" requirement and reuses a
  battle-tested pattern instead of inventing one.

**Web UI: React + TypeScript**, matching every other UI this operator has
built this session (kubespray-webui, compose-platform), for consistency
of operational knowledge, not because there's a technical reason Go's
ecosystem couldn't do it.

*This is the one decision in this doc most worth pushing back on if you
have a different constraint in mind (existing team expertise, an existing
Go-hostile platform standard, etc.) - flag it now, it's expensive to
reverse once workers and the plugin ABI are built against it.*

## 3. Direct challenges to the original spec

The brief asked explicitly to challenge bad decisions rather than
transcribe the list as-is. Here's where this doc disagrees with the
literal spec, and what it recommends instead:

### 3.1 "Every module exposes REST + gRPC + Event Interface" - no, not from day one

Building three parallel API surfaces for every one of ~40 modules from
day one triples the API maintenance and versioning surface before a
single real user exists. What actually earns its keep:

- **REST is the one stable, versioned, public contract** (OpenAPI-driven,
  used by the CLI, the Web UI, and third-party integrations). This is
  what Terraform Cloud, GitHub, and GitLab all converge on as *the*
  public API.
- **gRPC is internal-only**, specifically for Control-Plane-to-Worker
  communication (job dispatch, streaming logs, cancellation) where its
  bidirectional streaming and strong typing genuinely earn their
  complexity - not exposed as a second public API per module.
- **GraphQL is deferred**, and when it lands, it's a single **federated
  read-aggregation gateway** in front of the REST APIs (for the "graph"
  UI views - workspace graphs, infra topology - where GraphQL's
  strength, fetching a heterogeneous graph in one round trip, actually
  matches the UI's real access pattern), not a mandate on every module.
- **Domain Events go on the async event bus** (see below), which
  already *is* the event interface - a separate "Event Interface" per
  module on top of that would be redundant.

### 3.2 Drop the standalone Time-Series DB

The spec lists both Prometheus (under Observability) and "Time Series DB
if required" (under Database). Prometheus *is* the time-series database
for this system's metrics. Standing up a second one (InfluxDB/Timescale)
means either duplicating scrape targets or building a sync pipeline
between two TSDBs for no discernible product benefit. **Cut it** - if a
concrete need emerges later that Prometheus genuinely can't serve (e.g.
very-long-retention billing-grade usage metering), that's a narrow,
specific addition to design against real requirements then, not a
speculative platform-wide dependency now.

### 3.3 Search Engine: defer, don't build day-one

"Global Search" is a real, valuable feature - but standing up
OpenSearch/Meilisearch as core infrastructure on day one means every
future air-gapped install now also has to stage, size, and operate a
search cluster before the platform can be evaluated at all. Recommend:
**Postgres full-text search (`tsvector`/`pg_trgm`) for v1** (genuinely
adequate at the scale of "workspaces, runs, modules, resources" for any
single org that isn't already huge), with a dedicated search engine as a
**pluggable upgrade path** once real usage shows Postgres FTS is the
bottleneck - not a default core dependency.

### 3.4 Message bus: NATS (JetStream), not RabbitMQ

The spec lists "NATS or RabbitMQ" as an either/or - this doc picks NATS
JetStream specifically:

- Single small Go binary, no Erlang/OTP runtime to stage offline -
  directly relevant to the air-gap constraint above.
- JetStream gives durable streams + pull consumers, covering both the
  "fire a domain event" case and the "reliably queue a long-running job
  for a worker" case with one system instead of layering a job queue on
  top of a separate pub/sub broker.
- Matches the Go-first runtime decision (first-class Go client, written
  by the same people who write a lot of the Go infrastructure ecosystem
  this product sits alongside).

### 3.5 "Sentinel Compatibility" - reframe as OPA-only for the real policy engine

Already called out in the scope doc's non-goals: **OPA/Rego is the
policy engine**, full stop. A Sentinel-syntax translation shim is a
distinct, optional, and genuinely hard sub-project (Sentinel isn't
open-source; "compatibility" means reimplementing enough of its
semantics to run real Sentinel policies) - it does not belong in the
core architecture and should only be scoped if a real customer migration
requires it.

### 3.6 Multi-region / DR / HA: architect for it, don't build it in v1

These are real, correct requirements for an *enterprise* product, but
they're operational/deployment-topology concerns that a clean
stateless-Control-Plane-plus-Postgres-plus-object-storage architecture
already accommodates *if* the app layer stays stateless and all durable
state lives in Postgres/object storage/NATS JetStream (all of which have
their own real HA/multi-region stories). Recommend: **design so HA/DR is
"deploy topology," not "application code branch"** - i.e. don't let this
turn into a v1 feature checklist item; verify the stateless-app
assumption holds as each module is designed, and revisit multi-region
active-active specifically only once there's a real customer/latency
requirement driving it (it has real design cost - conflict resolution,
data residency - that shouldn't be paid speculatively).

## What this doc is asking you to confirm before Stage 2 (domain model)

1. **Go for Control Plane + Workers** - agree, or is there a constraint
   (team skillset, an existing standard) that should change this?
2. **Modular monolith + 2 satellite services** (Workers, Notification
   Dispatcher) as the v1 deployable shape - agree, or do you already know
   of a module that needs to be independently deployable/scalable from
   day one for a reason this doc hasn't accounted for?
3. **The five challenges in section 3** (API surface, TSDB, search
   engine, message bus, Sentinel) - confirm, or push back on any of them
   with the constraint this doc is missing.
4. **Primary initial deployment target**: which matters more to get right
   first - a single-org self-hosted install (Docker Compose, small team,
   likely air-gapped) or a multi-tenant SaaS-style install (Kubernetes,
   HA, many orgs)? Both are in-scope long-term per the spec, but the
   domain model's multi-tenancy boundaries (Org/Project/Team/Workspace
   isolation) get designed differently depending on which one is the
   *primary* target and which is the secondary one that has to still
   work.
