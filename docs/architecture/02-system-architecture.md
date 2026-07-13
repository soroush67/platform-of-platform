# System Architecture

Builds on `01-architecture-style-and-challenges.md`'s decisions (modular
monolith + Workers + Notification Dispatcher, Go, NATS JetStream,
Postgres, object storage). This doc is the system diagram, the
deployment topology for both target modes, and the control-plane/
data-plane split.

**Working assumption, carried forward from `01`'s open question 4**:
primary v1 target is a **self-hosted, single-org, air-gap-capable
install** (Docker Compose small-scale, Kubernetes/Helm for larger/HA) -
the same posture as every other project this operator has built this
session. Multi-org SaaS multi-tenancy is a real, designed-for future
target (the domain model in Stage 3 still gets Org-level isolation
right), but it's not what v1 deployment topology optimizes for. Flag
this now if it's wrong - it's the one assumption in this doc that
changes the diagram below if reversed.

## 1. High-level component diagram

```
                                   ┌─────────────────────────┐
                                   │        Web UI            │
                                   │   (React/TS, static)     │
                                   └────────────┬─────────────┘
                                                │ HTTPS
                     ┌──────────────────────────┼──────────────────────────┐
                     │                          │                          │
              ┌──────▼──────┐           ┌───────▼───────┐          ┌───────▼───────┐
              │     CLI     │           │  Public REST   │          │   Webhooks    │
              │ (Go binary) │──HTTPS───▶│      API       │◀────────│ (GitHub/Lab/  │
              └─────────────┘           │  (OpenAPI)     │         │  Bitbucket/…) │
                                         └───────┬────────┘         └───────────────┘
                                                 │
                                   ┌─────────────▼──────────────┐
                                   │        CONTROL PLANE         │
                                   │   (Go, modular monolith)     │
                                   │                               │
                                   │  Identity · Orgs/Projects/    │
                                   │  Teams · RBAC · Workspaces ·  │
                                   │  Environments · Registry ·    │
                                   │  Secrets metadata · Variables │
                                   │  · Policy (OPA) · Approvals · │
                                   │  Audit · GitOps orchestration │
                                   └───┬───────────┬──────────┬───┘
                                       │           │          │
                        gRPC (job     │  SQL       │  publish/consume
                        dispatch,      │  (system   │  (domain events,
                        log stream)    │  of record)│  job queue)
                                       │           │          │
                   ┌───────────────────▼──┐  ┌─────▼─────┐  ┌─▼──────────────────┐
                   │  EXECUTION WORKERS     │  │ PostgreSQL │  │  NATS JetStream     │
                   │  (Go, horizontally      │  │ (primary   │  │  (event bus + job   │
                   │  scaled pool)           │  │  datastore)│  │  queue, durable)    │
                   │                         │  └───────────┘  └──────────┬──────────┘
                   │  Plugin subprocess per                                │ consume
                   │  engine, RPC protocol:                                │
                   │  Terraform · OpenTofu ·                     ┌─────────▼─────────┐
                   │  Ansible · Helm · Compose ·                 │  NOTIFICATION       │
                   │  Packer · Kubespray ·                       │  DISPATCHER         │
                   │  native-Kubernetes                          │  (Go, stateless,    │
                   └──────────┬──────────────┘                  │  horizontally       │
                              │                                   │  scaled)            │
                    talks to real targets:                        │                     │
                    Git remotes, cloud APIs,                       │  Slack/Mattermost/  │
                    target VMs (SSH), K8s API                      │  Teams/Email/       │
                    servers, Vault API                             │  Webhook            │
                              │                                   └─────────────────────┘
                   ┌──────────▼──────────────────────────────┐
                   │       External systems this platform      │
                   │       orchestrates, does not replace:      │
                   │  Git hosts · HashiCorp Vault · Kubernetes   │
                   │  clusters (incl. ones we Kubespray'd) ·     │
                   │  Cloud provider APIs · Target VMs/hosts     │
                   └──────────────────────────────────────────┘

                   ┌─────────────────────────────────────────┐
                   │  Object Storage (S3-compatible / MinIO    │
                   │  for self-hosted): Terraform state files, │
                   │  plan output blobs, run logs, module/      │
                   │  provider registry artifacts               │
                   └─────────────────────────────────────────┘
                              ▲ read/write from both
                              └── Control Plane (metadata + presigned URLs)
                                  and Workers (actual blob I/O)

                   ┌─────────────────────────────────────────┐
                   │  Redis: cache, distributed locks           │
                   │  (workspace state locking), live log       │
                   │  tail pub/sub to the Web UI                │
                   └─────────────────────────────────────────┘

                   ┌─────────────────────────────────────────┐
                   │  Observability: Prometheus (metrics incl.  │
                   │  time-series), Loki (logs), Tempo/Jaeger    │
                   │  (traces), all via OpenTelemetry from every │
                   │  component above                            │
                   └─────────────────────────────────────────┘
```

## 2. Control plane vs. data plane

This split matters because it's the axis the whole security and scaling
story is built on:

- **Control plane** (the Control Plane service + Postgres + the public
  API): decides *what should happen* - who's allowed to do what, what a
  workspace's desired config is, whether a policy passes, whether an
  approval is needed. Never executes third-party code (a Terraform
  provider, an Ansible module, a Helm chart's hooks). Compromise of the
  control plane is bad (it's the source of truth for RBAC/secrets
  metadata) but it never directly runs code from a Git repo it doesn't
  own.
- **Data plane** (Execution Workers): does the actual work - runs the
  real `terraform apply`, the real `ansible-playbook`, the real `helm
  install`, against real infrastructure. This is where arbitrary
  third-party code (providers, modules, roles, charts) genuinely
  executes, so it's the component that gets the tighter execution
  sandbox (see `01`'s worker-isolation reasoning) and the narrowest
  credential scope (a worker gets a short-lived, run-scoped credential
  from Vault, never a standing one).

Everything in the "External systems" box is something the data plane
talks to on the control plane's behalf, never the reverse - the control
plane never opens a direct SSH/kubectl/cloud-API connection to a managed
target itself.

## 3. Deployment topology

### 3.1 Self-hosted single-org (v1 primary target)

```
docker-compose.yml (or a small single-node/3-node k3s if the org already
runs Kubernetes elsewhere and wants consistency):

  control-plane      (1-2 replicas, stateless behind the ingress)
  execution-worker    (N replicas, scaled by concurrent-run demand)
  notification-dispatcher (1-2 replicas)
  postgres            (single node to start; the same "operator can run
                       an HA Postgres later, this doesn't change the app"
                       posture as every other project this session)
  redis
  nats                (JetStream enabled, single node to start)
  minio               (S3-compatible object storage)
  prometheus + grafana + loki  (optional at first install, on by default
                       once the platform has real traffic worth watching)
```

Air-gapped install follows the exact pattern already proven in
kubespray-webui's Offline Install and vault-ha's `offline-repo/`: build
every container image and the CLI/worker plugin binaries once on a
machine with real internet access, ship the resulting bundle (images as
tars, or a private registry snapshot + a flat file bundle for
Go-binary-based plugins), `docker-compose up -d` on the air-gapped host
with zero outbound calls required at runtime.

### 3.2 Multi-tenant / HA (future target, architected for but not v1-optimized)

```
Kubernetes + Helm chart:

  control-plane        Deployment, HPA on request rate, PodDisruptionBudget
  execution-worker       Deployment, HPA on queue depth (NATS JetStream
                         consumer lag), can also run as Kubernetes Jobs
                         per-run for stronger isolation once that's the
                         real bottleneck to solve
  notification-dispatcher Deployment, HPA on request rate
  postgres               External managed Postgres (RDS/CloudSQL) or an
                         in-cluster HA operator (CloudNativePG) - not a
                         StatefulSet we hand-roll
  redis                  Managed or a small HA operator (same reasoning)
  nats                   NATS JetStream cluster (3+ nodes)
  object storage          Managed S3/GCS/Azure Blob, or in-cluster MinIO
                         in distributed mode for genuinely air-gapped HA
```

The point of designing both topologies now (not "Compose now, figure out
Kubernetes later") is that it forces the stateless-app-layer discipline
from `01`'s HA/DR challenge immediately - if a module design only works
when the Control Plane is a single process, that's caught at domain-model
time, not after a customer's HA install breaks.

## 4. Why NATS JetStream carries both "event bus" and "job queue"

Two conceptually different needs, one system, on purpose:

- **Domain events** (`workspace.created`, `run.completed`,
  `policy.violated`, ...): fire-and-forget-ish, potentially many
  subscribers (Notification Dispatcher, Audit Log writer, future
  webhook subscribers, the Web UI's live-event feed via a
  WebSocket-bridging subscriber). JetStream subjects with wildcard
  subscriptions cover this directly.
- **Job dispatch to Workers** (`run this Terraform plan`, `cancel this
  job`): needs at-least-once delivery, durable retry, and load-balanced
  consumption across a worker pool. JetStream's pull consumers with ack
  semantics cover this without a second broker.

Running two message systems (an event bus and a separate job queue) is
the more common pattern in older architectures mostly because most
brokers don't do both well - JetStream's design target genuinely covers
both, and consolidating avoids operating two stateful systems that would
otherwise both need HA, both need air-gapped packaging, and both need
their own monitoring.

## Open question before Stage 3 (domain model)

Confirming the working assumption stated at the top of this doc: **is
self-hosted single-org (Docker Compose primary, Kubernetes/Helm for
scale) really the right v1 target**, with multi-org SaaS as a
future-but-designed-for goal? This doc's whole deployment topology
section (and therefore a good chunk of Stage 3's Org/tenant isolation
design) is built on that assumption.
