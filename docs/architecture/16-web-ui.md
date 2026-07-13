# Web UI

First doc in Stage 0's post-module-detail sequence
(UI → Backend → Workers → Integrations → Tests → Deployment). Everything
here consumes the REST API from docs 07-14 and the real-time
infrastructure from Stage 5 §5/Stage 6 - this doc is about information
architecture and the handful of real technical decisions (state
management, real-time data flow, search), not visual design, which
belongs to an actual design pass once this structure is agreed.

## 1. Frontend stack

**React + TypeScript**, confirmed from Stage 1 §2 - consistency with
`compose-platform`'s own frontend this operator already built this
session, not a fresh evaluation. Two decisions Stage 1 left open,
resolved here:

- **Server state: TanStack Query, not a Redux-style global store for
  API data.** The overwhelming majority of this UI's state *is* server
  state (a Run's status, a Workspace's variables, the audit log) with a
  well-defined owner (the REST API) and a well-defined invalidation
  signal (the real-time event stream, §2) - TanStack Query's
  cache-plus-invalidation model matches that shape directly, while a
  global store would mean hand-writing the same cache/staleness logic
  TanStack Query already solved. A small amount of genuine *client*
  state (current theme, sidebar collapsed/expanded, in-progress form
  input) uses plain React state/context - deliberately not the same
  mechanism as server state, since conflating "data we own" with "data
  the server owns" is the actual source of most over-engineered
  frontend state management.
- **Routing: file-based, or explicit router config?** Explicit
  (`react-router` route definitions, matching `compose-platform`'s own
  `App.tsx` pattern this session) over a framework-driven file-based
  router (Next.js App Router, etc.) - this is a pure SPA behind the
  Control Plane's API and an auth gate (Stage 13), with no server-side
  rendering requirement (nothing here is public/SEO-relevant), so a
  full meta-framework buys rendering strategies this product doesn't
  need in exchange for real complexity it would need to opt back out of.

## 2. Real-time data flow

```
Worker ──gRPC status/log stream──▶ Control Plane
                                        │
                          ┌─────────────┼──────────────┐
                          ▼             ▼               ▼
                  Postgres write   Outbox event    Redis PUBLISH
                  (Stage 5)        (Stage 6)        (Stage 5 §5,
                                        │             live log lines
                                        ▼             only)
                              NATS JetStream
                                        │
                          WebSocket Gateway (a thin
                          Control Plane component -
                          just another JetStream +
                          Redis consumer, bridging
                          to browser WebSocket
                          connections, not a new
                          deployable per Stage 2's
                          "satellite services" list)
                                        │
                                        ▼
                                  Browser (this doc)
```

**Two distinct real-time channels, deliberately not unified into one**:
domain events (Run status changed, approval granted - durable, from
JetStream, drives cache invalidation) and live log lines (ephemeral,
from Redis pub/sub per Stage 5 §5, drives the log viewer's append-only
scroll). A single WebSocket connection per browser tab multiplexes
both (subject-based routing inside one socket, not one socket per
concern) - what's architecturally distinct is the *backing transport*
for each (durable JetStream vs. fire-and-forget Redis), not the
client-facing connection count.

**Consumption pattern in the frontend**: the WebSocket message handler
never directly mutates UI state - it calls
`queryClient.invalidateQueries(...)` (TanStack Query) for the affected
resource, letting the existing REST-fetch-and-cache path re-fetch and
re-render. This means every screen's "what does this look like" logic
lives in exactly one place (the query function against the REST API,
already fully specified in docs 07-14) - the WebSocket layer's only job
is "tell the cache this might be stale now," never "here's the new
data, render it," which would mean maintaining two different data
shapes (REST response vs. WebSocket payload) for the same resource.

## 3. Information architecture - top-level navigation mirrors the
## Stage 3 context map, not a flat feature list

```
Organization switcher (top-left, per Stage 4's org-rooted URL structure)
├─ Dashboard              live event feed + org-wide Run/Workspace summary (§4.1)
├─ Projects               → Environments → Workspaces (the actual work surface, §4.2)
├─ Clusters               Kubernetes context (Stage 12)
├─ Registry               Modules & Providers (Stage 12)
├─ Policies               Policy context (Stage 9)
├─ Approvals              cross-project inbox: "everything waiting on me" (§4.4)
├─ Git Connections        GitOps context (Stage 10)
├─ Secret Mounts          Secrets context (Stage 11)
├─ Audit Log              Audit context (Stage 14)
────────────────────────
├─ Team / People          Teams, Members, RBAC (Stage 13)
├─ Notification Channels  Stage 14
└─ Settings               Org settings, quotas, branding (Stage 3 §2)
```

Deliberately *not* one item per Stage 3 context in a flat list -
Workspace/Environment/Run collapse into one "Projects" drill-down (they
share one navigation story, per Stage 8's promotion flow), and Approval
gets **its own top-level item despite being a sub-resource of Run**
(Stage 9 §5's API is `/runs/{run}/approval`, never a standalone
collection) specifically because "show me everything waiting on my
decision, across every workspace" is a real, frequent cross-cutting
query no single Workspace-scoped screen answers - the nav structure and
the REST resource structure are allowed to diverge here on purpose, the
API stays resource-clean, the UI aggregates.

## 4. Key screens - only the ones with a real design decision, not
## every screen

### 4.1 Dashboard - a live-updating aggregate view

The one place the CQRS read model from Stage 6 §5 (`run_timeline_view`)
is the *primary* data source rather than falling back to direct REST
reads - specifically because "last 50 events across every workspace in
this org, with workspace/run names joined in" is exactly the query
shape Stage 6 justified a read model for, and re-deriving it from
per-workspace REST calls here would mean N requests for a screen meant
to load in one.

### 4.2 Workspace detail - the "hub" screen

Tabs, not separate pages, for: Runs (list + trigger), Variables
(Stage 8 §1's effective-resolved view, with source-scope shown inline),
State (versions, per Stage 11 §3), Settings (VCS link, lock status).
One screen because a user working on a workspace moves between these
constantly - separate page loads for each would be the wrong tradeoff
for how this screen actually gets used, even though each tab maps
cleanly to its own REST resource.

### 4.3 Run detail - the execution timeline

The screen the original spec's "Execution Timeline" item refers to
directly: a vertical timeline of the Run's Jobs (Stage 7 §1's phases -
plan, policy check, approval, apply) each expandable to its live/
completed log (§2's dual real-time channel), with the state-machine
transitions (Stage 7 §1) rendered as the timeline's own structure -
this screen **is** a visualization of that state machine, not a
separate design, which is exactly why Stage 7 §1 was worth specifying
as an explicit transition table rather than just a status enum: the UI
had a concrete diagram to render directly from.

### 4.4 Approvals inbox

Cross-project list (§3's nav rationale) - each entry links into the
owning Run's detail screen (§4.3) rather than duplicating the
approve/reject UI in two places; the inbox is a *filtered view into*
Run detail, not a parallel screen with its own decision-submission
logic.

### 4.5 Workspace/Infrastructure graph & Topology view

Two distinct spec items, two distinct data sources, worth
distinguishing rather than building one generic "graph screen": the
**Workspace graph** (Run Triggers from Stage 7 §3 - which workspaces
depend on which) is Control-Plane-resident data, a straightforward
directed graph render. The **Infrastructure/Topology view** (what
resources actually exist, per cluster/workspace) is fundamentally
different - it's a projection of a Run's plan/state output (real
resource data, Stage 11 §3's state version content), not a Control
Plane aggregate, so it's rendered from a specific state version's
parsed content on demand, not a live-updating dashboard widget the way
the Workspace graph is.

### 4.6 Global search

Per Stage 1 §3.3 and Stage 6 §5: Postgres full-text search for v1
(`tsvector` across workspace/run/module names and, where reasonable,
audit log free-text fields), a single search box, results grouped by
resource type. The UI is built against a `GET /api/v1/orgs/{org}/search
?q=...` endpoint from day one (not scattered per-context search boxes)
specifically so swapping the backend to a dedicated search engine later
(same Stage 1 deferred-upgrade path) is a server-side change with zero
frontend rework - the UI never knows or cares which engine answers the
query.

## 5. Theming, accessibility, keyboard shortcuts

- **Dark/light mode**: CSS custom properties + a `data-theme` attribute
  toggle (system-preference-aware default, explicit override
  persisted), not two separately maintained stylesheets - every
  component styled once, against theme-aware tokens.
- **Accessibility**: WCAG 2.1 AA as the explicit bar (not "best effort")
  - keyboard-navigable by construction (every interactive element a
  real `<button>`/`<a>`, not a `<div onClick>`), and the live log
  viewer/event feed specifically get `aria-live="polite"` regions,
  since a plain visual-only append-as-it-streams pattern is exactly the
  kind of UI that's silently unusable with a screen reader if not
  designed for it from the start.
- **Keyboard shortcuts**: a single global command palette (`Cmd/Ctrl+K`
  - opens global search, §4.6, doubling as the shortcut launcher) plus
  a small fixed set of navigation shortcuts (`g d` dashboard, `g p`
  projects, etc., the same "leader key" convention GitHub/Linear use) -
  not a large bespoke shortcut set invented per-screen, so shortcuts
  stay discoverable and memorable across the whole product rather than
  being a different vocabulary per page.

## Open questions before the next stage (Backend)

1. **Workspace detail tabs (§4.2)**: confirmed as one screen with tabs
   rather than separate routed pages - acceptable, or does deep-linking
   directly to (e.g.) a specific state version need its own URL rather
   than a tab-plus-query-param? (Easy to support either way, worth
   deciding the URL shape now rather than mid-implementation.)
2. **Ready for Backend stage next** (server-side implementation
   structure: Go package layout mapped to the Stage 3 bounded contexts,
   the hexagonal ports/adapters boundaries Stage 1 committed to,
   dependency injection wiring) - confirm, or Workers first instead
   (the execution-engine plugin protocol, actually the more novel and
   higher-risk piece of this whole system) - both are valid next steps
   and this doc set has been consistently asking rather than assuming
   the order from here on.
