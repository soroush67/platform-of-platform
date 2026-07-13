# Module detail: Secrets & State Management

Fifth module doc, done together per the plan from Stage 10: both are
"what a Job actually needs to run, and what it produces," the two
concrete inputs/outputs every executed Run touches regardless of engine.

## 1. REST API - SecretMount

`/api/v1/orgs/{org}/secret-mounts`

```json
// POST
{
  "backend_type": "vault",              // vault | aws_secrets_manager | azure_keyvault | gcp_secret_manager
  "connection_config": {
    "address": "https://vault.internal:8200",
    "auth_method": "approle",
    "role_id": "...", "secret_id": "..."   // write-only, itself stored via a bootstrap credential - see §2
  }
}
```

**A real bootstrapping question this doc has to answer, not gloss
over**: the Control Plane needs *some* credential to reach the Vault
instance in `connection_config` in order to later request short-lived,
run-scoped tokens on a Worker's behalf (Stage 8 §4). That bootstrap
credential (the AppRole `secret_id` above, or equivalent) is itself a
secret with no further backend to defer to - it's encrypted at rest in
Postgres using an envelope-encryption scheme, the exact pattern this
operator already built and proved this session in `compose-platform`
(a master key from environment/KMS, per-record keys derived via
BLAKE2b, AES-GCM) and in `vault-ha` (the same envelope pattern for its
own webhook URL/SMTP password settings) - reused here rather than
designed fresh, since it's already a proven, tested approach for
exactly this "one secret has to live in our own database because
there's no further backend to point at" problem.

### `POST /secret-mounts/{mount}/test-connection`

Verifies the stored credential can actually authenticate and list at
least the mount's root path, without revealing any secret content -
same "let an operator verify a credential before committing to using
it" pattern as `compose-platform`'s own `POST
/machines/test-connection` this session, generalized to secret backends.

## 2. Where SecretReference actually shows up (no independent CRUD -
## by design)

Per Stage 3 §8, `SecretReference` is a **value object**, not an
aggregate - there is no `POST /secrets` endpoint. It only ever appears
embedded inside a Variable (Stage 8 §1: `{"key": "db_password", "value":
{"secret_ref": {"mount_id": "...", "path": "database/prod/password"}}}`)
or, later, inside engine-specific config (a Kubernetes Cluster's
`kubeconfig_secret_ref`, Stage 3 §13). This is deliberate: giving
SecretReference its own top-level API would imply it has identity and
lifecycle independent of whatever's using it, which isn't true - a
reference that nothing points to isn't a resource this platform tracks,
it's just an unused path in someone else's Vault.

## 3. REST API - StateVersion

`/api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}/state-versions`

Read-only from the API's perspective (`GET` list, `GET` single) -
**state versions are never created via a direct API call**, only ever
as a side effect of a Run's apply Job completing successfully (Stage 7
§3's Worker writes the object storage blob and the Control Plane
inserts the `state_versions` row in the same transaction that
transitions the Run to `applied`, per Stage 6's outbox pattern - the
`state_version.created` event and the `run.completed` event both ride
the same outbox commit). This is worth stating explicitly because
Terraform Cloud's own API *does* expose a "push a state version
directly" endpoint (for migrating existing local state in) - this
platform's answer to that same real need is a **one-time import flow**
(§5), not a standing general-purpose "overwrite state via API"
capability, because the latter is exactly the kind of endpoint that
turns "state is only ever produced by an audited apply" into "state can
also silently diverge from what was actually applied," undermining the
whole reason state versioning exists.

### `GET /state-versions/{version}/download`

Same presigned-URL-redirect pattern as Run outputs (Stage 7 §2) - never
proxied through the Control Plane.

### `GET /state-versions/latest`

Convenience alias for the dominant query Stage 5's index
`(workspace_id, serial desc)` was built for.

## 4. State locking - confirmed as *not* a separate mechanism

Restating Stage 5 §2's decision explicitly here since this is the doc
where someone designing "state management" would otherwise reach for a
dedicated lock table by habit (that's literally what Terraform's own
remote-backend protocol does - a `lock`/`unlock` HTTP verb pair against
a state backend): **this platform doesn't need a separate state lock**,
because the *Workspace* lock (Stage 5 §2, Stage 7 §1's non-terminal
Run statuses) already guarantees at most one Run can be mutating a
workspace's state at any time - a second, independent state-specific
lock would just be tracking the same fact twice, with the two copies
now able to drift.

## 5. State import (the one-time migration path from §3)

`POST /workspaces/{workspace}/state-versions/import`

```json
{ "state_content": "<raw terraform state JSON, base64>" }
```

Explicitly gated: `403` unless the workspace has **zero** existing state
versions (this is a bootstrap-only operation for bringing existing,
already-real infrastructure under this platform's management - not a
general write path). Creates exactly one `state_versions` row with
`created_by_run_id: null` and a distinct `import` provenance flag,
which the Web UI surfaces clearly (a state version's history should
never let "was this actually applied through the platform, or imported
from an existing local state file" be ambiguous later).

## Open questions before the next module doc

1. **Multi-backend secret resolution within one Workspace**: Stage 3
   scoped `SecretMount` to an Organization, and a Workspace's Variables
   can each reference a different mount - confirmed this is fine (a
   Workspace can pull one secret from Vault and another from AWS
   Secrets Manager in the same Run), or should a Workspace be
   restricted to secrets from a single mount for operational simplicity?
2. **Which module next?** Recommend **Kubernetes** (the context with
   the most spec-listed sub-features - node inventory, Cilium, MetalLB,
   Longhorn - that Stage 3 deliberately kept thin "until the per-module
   stage," which is now) paired with **Registry** (Module/Provider),
   since both are "this platform indexes/manages metadata about
   something whose real state lives elsewhere" - Kubernetes' real state
   lives in the cluster's own API server, Registry's real content lives
   in object storage, matching pattern to §3's own "read-mostly,
   synced, not the source of truth" framing.
