# Workers - execution engine plugin architecture

Second doc in the post-module sequence, deliberately chosen ahead of
Backend structure: this is the highest-uncertainty, highest-risk piece
of the whole system (every other doc's Job dispatch, credential
handoff, and log streaming assumptions - Stage 4 §10, Stage 7 §3, Stage
8 §4 - were all written *against* this doc's design before it existed;
better to find out now if any of those assumptions don't actually hold).

## 1. Worker process shape

A Worker is one Go binary (per Stage 1 §2) with two responsibilities:
maintain the long-lived gRPC `StreamJobs` connection to the Control
Plane (Stage 4 §10), and, for each `JobAssignment` it receives, launch
and supervise a **plugin subprocess** that does the actual engine-
specific work. The Worker itself never runs Terraform/Ansible/Helm code
directly - it's a supervisor, matching the reasoning in §4 for why
that separation matters.

```
Worker (long-lived process)
  │
  ├─ gRPC client: StreamJobs (receives JobAssignment)
  │              ReportJobStatus (sends status updates)
  │              StreamJobLogs (sends log chunks)
  │
  └─ per JobAssignment: launch ── Plugin subprocess (per §2)
                                      │
                                      └─ shells out to the real
                                         terraform/ansible-playbook/
                                         helm/docker/packer/ansible
                                         (kubespray) binary
```

## 2. Plugin protocol - local RPC subprocess, per Stage 1 §2's decision

Each engine type (`terraform`, `opentofu`, `ansible`, `helm`,
`compose`, `packer`, `kubespray`, `kubernetes`) is a **separate plugin
binary**, launched by the Worker as a child process communicating over
a local Unix socket (gRPC again - one protocol for both
Worker-to-Control-Plane and Worker-to-Plugin, not two different RPC
mechanisms to maintain). Same pattern Terraform/Vault/Packer's own
plugin systems use, reused rather than invented:

```protobuf
service EnginePlugin {
  rpc Validate(JobInput) returns (ValidateResult);
  rpc Plan(JobInput) returns (stream ExecutionEvent);    // ExecutionEvent: {log_line} | {progress} | {plan_output} | {done, exit_code}
  rpc Apply(JobInput) returns (stream ExecutionEvent);
  rpc Destroy(JobInput) returns (stream ExecutionEvent);
  rpc Cancel(JobID) returns (Ack);
}

message JobInput {
  string job_id;
  bytes  config_bundle;        // the workspace's source (Git checkout, or an uploaded Compose file - engine-specific interpretation)
  map<string,string> variables; // already-resolved plain values (Stage 8 §3's resolution already happened Worker-side, before this call)
  repeated SecretHandle secrets; // NOT resolved values - references the plugin resolves itself, see §5
}
```

**Why every engine implements the same four RPCs rather than the spec's
longer list (Plan/Validate/Preview/Execute/Rollback/Destroy) verbatim**:
`Preview` and `Plan` are the same operation under different names
across engines (Terraform calls it plan, Helm calls it `--dry-run`,
Ansible has `--check` mode) - normalized to one verb, `Plan`, with each
plugin translating it to its own engine's real equivalent internally.
`Execute` is `Apply`. `Rollback` is **not a fifth RPC** - per Stage 7
§1's state machine, "undo a bad apply" is a new Run (typically applying
an earlier, known-good state/config version), not an operation this
protocol needs its own verb for, the same reasoning Stage 11 §5 already
applied to state ("state only changes via a real, audited apply, never
a direct write").

## 3. Not every engine implements every RPC meaningfully - and that's
## fine, not a protocol failure

`Docker Compose` has no real "plan" concept (there's no
`docker compose plan`) - its `Plan` RPC returns a best-effort diff
(compare the target Compose file against what's currently running on
the target host via SSH, the same mechanism this operator's own
`compose-platform` already uses for its rendered-config preview this
session) rather than a true dry-run. `Packer` has no meaningful
`Destroy` (an image build has nothing to tear down) - its plugin
returns a clean no-op success rather than an error, so the Run state
machine (Stage 7 §1) doesn't need an engine-specific branch for "this
engine doesn't support destroy," it just always completes trivially
fast. **The protocol is uniform; the semantic depth behind each RPC is
allowed to vary per engine** - forcing every engine into an
artificially rich plan/destroy model it doesn't really have would be
worse than a protocol that's honest about which engines have thin
implementations of which verbs.

## 4. Job isolation - containers now, a documented upgrade path to
## stronger isolation later

**The real security question Stage 2 raised but didn't answer**: a
`terraform apply` runs arbitrary third-party provider code, an Ansible
playbook runs arbitrary role code, a Helm chart can run arbitrary hook
code - genuinely untrusted, especially anything sourced from the public
registry rather than an org's own vetted modules. This doc picks a
concrete baseline rather than leaving it open:

- **v1 (matches Stage 2's self-hosted-single-org primary target)**:
  each Job's plugin subprocess runs inside an **ephemeral OCI
  container** (the Worker itself manages this via the Docker/Podman
  API on its host, or as a Kubernetes Job/Pod when the Worker itself
  runs in Kubernetes per Stage 2 §3.2), with CPU/memory limits, no
  inbound network exposure, and only the outbound network access the
  specific engine genuinely needs (a Terraform apply needs to reach
  cloud provider APIs; nothing needs to reach the Worker's own
  filesystem outside its job workspace). Destroyed immediately on Job
  completion - no state persists in the container itself, only in
  object storage (Stage 5 §4) and the returned RPC results.
- **Documented, not built, upgrade path**: microVM-level isolation
  (Firecracker, gVisor) for the multi-tenant SaaS target (Stage 2
  §3.2) where "untrusted code from Org A's Terraform module" and "Org
  B's Job running on the same physical Worker fleet" is a real,
  adversarial threat model container isolation alone doesn't fully
  close - **flagged here explicitly as a gap for that specific target,
  not silently assumed equivalent to the self-hosted case** where the
  operator is typically running their own trusted-enough infrastructure
  code and the isolation boundary matters less.

## 5. Credential handoff - the concrete mechanics behind Stage 8 §4's
## boundary statement

1. Run Dispatcher (Stage 7 §3) resolves the Job's `variables` -
   including any `SecretReference`s (Stage 8 §1) - but for secrets,
   instead of a value, requests a **short-lived, path-scoped token**
   from the relevant `SecretMount`'s backend (e.g. a Vault batch token
   restricted to exactly the referenced path, TTL matching the Job's
   configured timeout from Stage 7 §3 plus a small grace margin).
2. That token (not the underlying secret value) rides inside the
   `JobAssignment` as a `SecretHandle` (§2's `JobInput.secrets` field) -
   the Control Plane never touches the actual secret content, only
   requests and forwards a credential that's *itself* useless for
   anything beyond that one path within that one TTL window.
3. **The plugin subprocess**, running inside the Job's container (§4),
   uses that token to fetch the actual value directly from the backend
   at the moment it's needed (injected as an environment variable or a
   generated `.tfvars`/Ansible-vars file inside the container's
   ephemeral filesystem only) - this is the exact moment Stage 8 §4
   called "the Worker resolves it," now specified as "the plugin,
   inside the isolated container, using a token good for nothing else."
4. Token expires at Job completion regardless of whether it was
   explicitly revoked - defense in depth against a Worker or plugin bug
   that fails to clean up.

## 6. Cancellation and retry - explicit, not implied by the RPC names

**Cancellation**: `POST /runs/{run}/cancel` (Stage 7 §2) → Control
Plane's Run Dispatcher calls the Worker's `Cancel(job_id)` RPC over the
already-open connection → Worker sends `SIGTERM` to the plugin
subprocess's entire process group (not just the immediate child - a
`terraform apply` spawns provider subprocesses of its own that need to
receive the signal too), waits a configurable grace period (default
30s, letting e.g. Terraform attempt a clean interrupt rather than
leaving a resource half-created), then `SIGKILL` if it hasn't exited.
The container itself (§4) is destroyed regardless of clean-exit success,
guaranteeing no orphaned process survives a cancel even if signal
handling inside the plugin is imperfect.

**Retry is always a new Run, never a silent automatic re-attempt.** The
spec listed "Retry" as an engine capability; this doc deliberately does
**not** build automatic retry-on-failure into the plugin protocol or
the Run state machine - a failed `apply` may have partially succeeded
(some resources created, others not), and blindly re-running the exact
same operation risks resource-conflict errors or, worse, silently
succeeding in a way that masks that the first attempt left something
in a bad state. `POST /runs/{run}/retry` exists (creates a new Run with
`retried_from_run_id` set, same config/variables) but it's **always an
explicit, human- or API-triggered action**, going through the full
state machine (plan again first, not a blind re-apply) - retry-as-a-
deliberate-decision, not retry-as-automatic-resilience, is the
considered position here given what's actually at stake when the thing
being retried mutates real infrastructure.

## 7. Worker registration & pool matching (fills in Stage 7 §3's Run
## Dispatcher reference)

At startup, a Worker's `WorkerRegistration` (Stage 4 §10) advertises:
`supported_engines: [terraform, ansible, ...]` (which plugin binaries
are actually installed/available - relevant for air-gapped installs
where an org might only stage the engines they actually use, per Stage
1's constraint) and `labels: {region: us-east, size: large, ...}`
(operator-assigned, matching the spec's "Worker Affinity" requirement).
The Run Dispatcher's claim query (Stage 7 §3) becomes: find a Worker
whose `supported_engines` includes the Workspace's `execution_engine`
and whose `labels` satisfy any affinity rules configured on the
Workspace/Environment - a straightforward label-selector match, the
same mental model as Kubernetes node affinity, chosen because it's
already a well-understood pattern rather than a bespoke matching DSL.

## Open questions before the next stage (Backend)

1. **v1 container runtime dependency**: §4 assumes the Worker's host
   has Docker/Podman (self-hosted) or runs as Kubernetes Jobs
   (Kubernetes deployment) - confirm neither target needs to support
   "Worker with no container runtime available at all" (a bare-metal-
   process-only fallback), which would need a materially different,
   weaker isolation story.
2. **Ready for Backend stage**: with Workers' plugin protocol and
   isolation model now concrete, the Backend doc can specify the actual
   Go package/module layout implementing everything from docs 03-17 -
   confirm proceeding there next.
