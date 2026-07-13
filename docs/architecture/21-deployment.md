# Deployment

Thirteenth and final doc in Stage 0's original sequence - where every
decision across docs 00-20 becomes an actual deployment manifest.
Confirms and makes concrete the two topologies Stage 2 sketched.

## 1. Docker Compose (self-hosted, v1 primary target)

```yaml
services:
  control-plane:
    image: platform/control-plane:${VERSION}
    environment:
      DATABASE_URL: postgres://...
      NATS_URL: nats://nats:4222
      REDIS_URL: redis://redis:6379
      OBJECT_STORAGE_ENDPOINT: http://minio:9000
      MASTER_KEY: ${MASTER_KEY}           # envelope encryption root, Stage 11 SS1 - same bootstrap-secret posture as compose-platform/vault-ha this session
      INITIAL_PLATFORM_ADMIN_EMAIL: ${INITIAL_PLATFORM_ADMIN_EMAIL}
    depends_on: [postgres, nats, redis, minio]
    ports: ["8443:8443"]                   # HTTPS only, per Stage 4 - no plaintext HTTP listener in this binary at all

  worker:
    image: platform/worker:${VERSION}
    deploy: { replicas: 3 }                # scale independently of control-plane, per Stage 2 SS2
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock   # DooD, for launching Job containers per Stage 9 SS4 - same pattern this operator's kubespray-webui/compose-platform already use this session for their own offline-build and deploy-execution containers
    environment:
      CONTROL_PLANE_GRPC_ADDR: control-plane:9000
      WORKER_LABELS: "region=local,size=default"    # Stage 9 SS7 affinity labels

  notification-dispatcher:
    image: platform/notification-dispatcher:${VERSION}
    deploy: { replicas: 2 }

  postgres:
    image: postgres:16
    volumes: [postgres-data:/var/lib/postgresql/data]

  nats:
    image: nats:2-alpine
    command: ["-js"]                        # JetStream enabled, Stage 2 SS4
    volumes: [nats-data:/data]

  redis:
    image: redis:7-alpine

  minio:
    image: minio/minio
    command: server /data
    volumes: [minio-data:/data]

  # observability stack (Stage 1 SS3.3/Stage 11 SS3) - optional profile,
  # on by default once there's real traffic worth watching, per Stage 2's
  # own framing, not a hard requirement for first bring-up
  prometheus:
    profiles: ["observability"]
  grafana:
    profiles: ["observability"]
  loki:
    profiles: ["observability"]
  tempo:
    profiles: ["observability"]
```

`control-plane` and `worker` are genuinely separate images (Stage 2's
control-plane/data-plane split isn't just a logical boundary, it's a
real image boundary - the Worker image includes the plugin binaries
(terraform/ansible/helm/packer/kubespray's ansible collections), the
Control Plane image doesn't need any of them and shouldn't have the
attack surface of bundling execution engines it never runs itself.

## 2. Kubernetes / Helm (HA / future multi-tenant target)

```
helm install platform ./chart --values values-production.yaml

chart/templates/
  control-plane-deployment.yaml    HPA on request rate, PodDisruptionBudget
  worker-deployment.yaml            HPA on NATS JetStream consumer lag (Stage 2 SS3.2) - or,
                                    once real load data justifies the
                                    stronger isolation, a Job-per-Run
                                    mode instead of a long-lived
                                    Deployment pool (flagged, not built,
                                    same "don't pre-build for scale you
                                    don't have" posture as every other
                                    deferred item in this doc set)
  notification-dispatcher-deployment.yaml
  networkpolicy.yaml                 control-plane <-> worker only via
                                    the gRPC port; worker has no inbound
                                    rule at all (Stage 4 SS10's whole
                                    point - workers only ever dial out)
  servicemonitor.yaml                 Prometheus Operator integration
values.yaml:
  postgres.external: true/false       external managed Postgres, or
                                    bundled CloudNativePG (Stage 2 SS3.2)
  objectStorage.external: true/false  managed S3, or bundled MinIO
                                     distributed mode
```

## 3. Air-gapped install - the proven pattern, not a new one

**Exactly the build-once-with-real-internet, ship-the-bundle,
zero-outbound-calls-at-runtime pattern already proven this session in
kubespray-webui's Offline Install and vault-ha's `offline-repo/`** -
applied here to container images instead of apt packages: `docker save`
every image referenced above into a tarball (or push to a private
registry snapshot), bundle the Worker's plugin binaries (each a real
static Go/HashiCorp binary, fetched and verified once with real
internet access, per Stage 1's air-gap constraint applying to
*everything* the Worker needs, not just the platform's own code), ship
the bundle to the air-gapped host, `docker load` + `docker-compose up
-d` with zero outbound calls required. This doc doesn't re-derive that
pattern, it confirms the same technique this operator has now used
across three separate projects this session applies unchanged here.

## 4. Bootstrap sequence

```
1. docker-compose up -d postgres nats redis minio
2. control-plane runs `golang-migrate up` on first start (Stage 5 SS3) -
   same "the app migrates itself on startup" convention as this
   operator's compose-platform/mongodb-cluster/vault-ha projects this
   session, not a separate manual migration step an operator has to
   remember
3. control-plane seeds: Permission enum (Stage 13 SS3's fixed set),
   built-in Roles (owner/admin/write/read), and - only if
   INITIAL_PLATFORM_ADMIN_EMAIL is set - a platform_admin User awaiting
   first OIDC/local login, exactly mirroring compose-platform's own
   INITIAL_ADMIN_USERNAME/PASSWORD seed-on-first-start pattern this
   session, generalized from "seed one org's admin" to "seed the one
   platform_admin who can then create the first real Organization"
   (Stage 13 SS4's open question about self-service org creation still
   applies here - if self-serve, this step is optional; if
   admin-gated, it's how the very first org ever gets created)
4. docker-compose up -d control-plane worker notification-dispatcher
```

## 5. Backup / restore - three systems, three real strategies, no
## custom backup tooling

- **Postgres**: standard `pg_dump`/WAL archiving (or the managed
  service's own backup feature in the Kubernetes/managed target) - it's
  the system of record (Stage 5), so this is the backup that actually
  matters most and gets the least novel treatment on purpose.
- **Object storage**: bucket versioning + cross-region/cross-host
  replication where the deployment target supports it (MinIO's own
  replication for self-hosted, native for managed S3/GCS) - state
  versions and Run logs living here (Stage 5 §4) means this is the
  second most important backup target, not an afterthought because it's
  "just blobs."
- **NATS JetStream**: explicitly **not** a backup target in the
  traditional sense - per Stage 6, it's a notification/dispatch
  mechanism over facts already durably recorded in Postgres via the
  outbox, not a store of unique data; losing JetStream's own stream
  history loses *in-flight* event delivery state, recoverable by the
  Outbox Relay's `published_at IS NULL` reconciliation (Stage 6 §2), not
  by restoring a JetStream backup that doesn't need to exist as a
  distinct discipline.

## 6. Rolling updates

Control Plane and Notification Dispatcher: standard rolling deployment
(stateless, per Stage 2's HA discipline - any instance can serve any
request). **Workers need a drain step**, not a plain rolling replace: a
Worker mid-Job holds a `SIGTERM` differently than the Control Plane does
(Stage 9 §6's cancellation grace period applies to an *operator-
requested* cancel; a deploy-triggered restart should instead let
in-flight Jobs finish naturally, not cancel them) - the deploy process
marks a Worker `draining` (stops accepting new `JobAssignment`s over its
`StreamJobs` connection, per Stage 4 §10, but keeps its existing
in-flight Job running to completion) before actually terminating it,
avoiding a routine platform upgrade silently canceling someone's
long-running Kubespray apply.

## 7. Licensing (the spec's remaining item, deliberately thin here)

A signed license token (JWT, verified by the Control Plane at startup
and periodically) gating **enterprise features only** (SAML/LDAP per
Stage 11's IdP integrations, audit-export per Stage 14, multi-region -
if/when built) - **not** gating core functionality (a Run, a Workspace,
basic RBAC all work unconditionally). This doc doesn't design the
license-issuance/billing system itself (Stage 1's explicit non-goal,
still deferred - no real pricing model exists yet to design a licensing
system against) - only confirms the *enforcement point* (one check at
Control Plane startup + periodic re-check, feature-flag-shaped, not
scattered throughout the codebase) so a future licensing system has one
place to plug into rather than needing every context's code
retrofitted.

## Closing: Stage 0's originally-planned sequence is now complete

`00` through `21` cover vision → architecture style → system design →
domain model → APIs → database → events → all 16 modules → the
cross-cutting pattern catalog → UI → backend structure → workers →
integrations → tests → deployment. Every stage got explicit sign-off
before the next began, every module doc resolved or explicitly deferred
its own open questions, and every claim that was checkable against real
behavior (RLS isolation, OPA sandboxing, the outbox pattern, the
Kubespray-cluster-registration flow) got named as something to actually
verify, not just asserted.

**What's genuinely next, beyond this architecture phase**: real
implementation, starting from whichever bounded context you want to
build first (Stage 10 §2's per-context structure means each one is
independently buildable/reviewable once its own module doc - 07 through
14 - exists) - or a return pass over any of these 21 docs if something
earlier now looks different in light of a later decision (the doc set's
own open-questions sections are exactly where that kind of revision
would start).
