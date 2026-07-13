# Module detail: Kubernetes & Registry

Sixth module doc, paired per Stage 11's closing note: both contexts are
"index/manage metadata about something whose real, authoritative state
lives elsewhere" (a live cluster's API server; object storage holding
module/provider content) rather than owning a rich transactional domain
model of their own.

## Part A: Kubernetes

### 1. REST API - Cluster

`/api/v1/orgs/{org}/clusters` (optionally further scoped under a
Project, per Stage 3 §13's nullable `project_id`)

```json
// POST - two real registration paths, one endpoint
{
  "name": "prod-us-east",
  "provisioning_source": "imported",         // imported | eks | gke | aks - kubespray_managed is NEVER posted directly, see §2
  "kubeconfig": "<raw kubeconfig yaml>"       // write-only; stored as a SecretMount + SecretReference (Stage 11 §1-2), not inline
}
```

`kubespray_managed` clusters are not created through this endpoint at
all - see §2, they're a *result* of a Run, not a manually-registered
fact.

### `GET /clusters/{cluster}/nodes`

Returns the last-synced node inventory (§3) - explicitly documented in
the response as a cache with a `synced_at` timestamp, so the Web UI (and
any API consumer) never mistakes "what this endpoint returns" for "the
live state of the cluster right now."

## 2. How a `kubespray_managed` Cluster actually comes into existence

A Kubespray provisioning run is simply a Workspace with
`execution_engine: kubespray` (Stage 3 §5's existing enum) - nothing new
at the Workspace/Run level. What's new: **on that specific engine type's
apply Job succeeding, the Worker extracts the generated kubeconfig
(Kubespray's own playbook output) and the Control Plane, reacting to
`run.completed` for a `kubespray`-engine Run, automatically creates the
Cluster (`provisioning_source: kubespray_managed`) and its backing
SecretMount/SecretReference** - the same outbox-transaction pattern as
State Management's `state_version.created` (Stage 11 §3): one Run
produces both a state version (the infrastructure/VM layer Terraform
managed underneath, if that's how the VMs themselves were provisioned)
*and* a Cluster registration (the Kubernetes layer Kubespray configured
on top), and both are equally "this Run's real output," not one primary
and one bolted-on side effect.

## 3. Health check & node inventory sync - a job type Execution doesn't own

**A real design gap this doc has to close rather than wave at Stage 3's
"synced by a periodic Job" note**: syncing node inventory means calling
the cluster's real API server, which means resolving the cluster's
kubeconfig secret - and per Stage 2/8's boundary, **the Control Plane
never resolves a secret value itself**. This can't be a Control-Plane-
internal timer calling out directly.

**Resolution**: a lightweight **Cluster Sync Job**, dispatched through
the exact same Worker gRPC mechanism as any other Job (Stage 4 §10) -
but explicitly *not* wrapped in a Run/Workspace/state-machine (Stage 7
§1 doesn't apply here; there's no plan, no approval, nothing to audit as
an infrastructure *change*, just a read). A small Kubernetes-context
scheduler (parallel to Execution's Run Dispatcher, but its own,
simpler loop - not a variant of it, since it dispatches on a fixed
interval per Cluster rather than draining a queue) sends a minimal job
payload (`{cluster_id, kubeconfig_secret_ref}`) to any Worker advertising
`kubernetes` capability; the Worker resolves the secret itself (same as
any other Job), calls the cluster's API server for node list +
`/healthz`, and reports back just the summary - Control Plane stores it
as the `nodes`/`health_status` cache, nothing more.

This is also where the spec's Cilium/MetalLB/Longhorn/cert-manager
inventory items live at this level of detail: each is a **read from the
same Cluster Sync Job** (list the relevant CRDs/resources via the
already-open kubeconfig-authenticated client), not a separate job or
separate credential resolution per add-on - one sync pass, several
things read from the same live connection.

## 4. What Kubernetes cluster *operations* (upgrade, backup, Cilium
## install) actually are

Not new Cluster-context endpoints - they're **Runs**, on a Workspace
whose `execution_engine` targets the operation (an `ansible`-engine
Workspace running the platform's own Cilium-install playbook against
the Cluster's kubeconfig, a `kubespray`-engine Workspace re-run for an
upgrade). This deliberately keeps "anything that changes cluster state"
inside the audited, policy-checked, approval-gated Run lifecycle from
Stage 7 - Kubernetes-context-owned endpoints stay read-only (registration
+ inventory), exactly mirroring how this doc's own opening framed the
context: metadata about a cluster, not a second execution path that
bypasses Execution's own governance.

## Part B: Registry

### 5. Terraform Registry Protocol compatibility - the actual design decision

**Rather than inventing a bespoke Module/Provider API, this platform
implements HashiCorp's published, open Registry Protocol** (the same
`/v1/modules/{namespace}/{name}/{system}/versions`,
`/v1/providers/{namespace}/{type}/versions`,
`.well-known/terraform.json` discovery endpoint) that `terraform init`
and `tofu init` already speak natively. Concrete payoff: pointing an
existing Terraform config at this platform's registry needs zero client-
side plugin or special configuration beyond the standard
`source = "platform-host/org/module-name/system"` syntax Terraform
already supports for any private registry - this platform becomes a
drop-in private registry, not a separate thing teams have to learn a new
protocol for. The platform's own management endpoints (creating a
Module, publishing a version) are a separate, platform-specific API
surface (`/api/v1/orgs/{org}/modules`) alongside the protocol-compliant
one - the protocol endpoints are what `terraform init` talks to, the
management ones are what this platform's own Web UI/CLI talk to.

### 6. REST API - Module (management surface)

```json
// POST /api/v1/orgs/{org}/modules
{
  "namespace": "networking", "name": "vpc", "target_system": "aws",
  "source": {
    "git_connection_id": "uuid", "repo": "terraform-modules",
    "path": "modules/vpc"
  }
}
```

**Versions are published by Git tag, not upload** - matching the public
Terraform Registry's own model and this platform's "Everything as Code"
posture (Stage 1's non-goals already ruled out reimplementing engines;
consistently, this doc rules out a bespoke tarball-upload flow as the
primary path). A GitOps-context webhook (Stage 10) on tag push matching
a configured pattern (default `v*`) triggers a **Module Version
Ingestion Job** (same "lightweight Worker job outside the Run state
machine" shape as §3's Cluster Sync) that clones the tag, computes a
checksum, and writes a `ModuleVersion` row + the packaged source into
object storage (`modules/{namespace}/{name}/{version}.tar.zst`, per
Stage 5 §4's layout).

Providers follow the identical shape (`/api/v1/orgs/{org}/providers`,
protocol-compliant `/v1/providers/...` endpoints) - not written out
separately since every design decision above (protocol compatibility,
Git-tag-sourced versions, ingestion-job-not-upload) applies unchanged.

## Open questions before the next module doc

1. **Cluster Sync interval and failure backoff**: default sync interval
   (this doc assumes something like 60s for health, less frequent for
   full node inventory) and what happens after N consecutive sync
   failures (mark `health_status: unreachable` after how many misses?) -
   real operational tuning worth a number, not left implicit.
2. **Which module next?** Recommend closing out with the three
   remaining contexts that are lowest-risk/most-mechanical at this
   point given everything above already exercises their interfaces
   heavily - **Identity & Access + RBAC + Tenancy together** (their
   full CRUD API was never given its own doc despite being referenced
   from every other module), then **Audit + Notifications** together
   (the write side was fully specified in Stage 6, what's left is
   mostly their own read/query API), which would complete all 16
   contexts.
