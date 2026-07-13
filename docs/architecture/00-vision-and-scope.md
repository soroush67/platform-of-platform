# Vision & Scope

## What this is

A unified Infrastructure & Platform Engineering platform: one control plane
that takes an organization from "we have Terraform, Ansible, Helm, Vault,
and Kubespray scattered across scripts and tribal knowledge" to "one web UI,
API, and CLI govern every stage of infrastructure delivery" - provisioning
(Terraform/OpenTofu), configuration (Ansible), packaging (Docker
Compose/Helm/Packer), cluster lifecycle (Kubespray/Kubernetes), secrets
(Vault), GitOps delivery, policy, and audit.

It deliberately overlaps, in narrow slices, with Terraform Enterprise,
Spacelift, Scalr, Backstage, Port, Rancher, Argo CD, and AWX - but the
product is the *unification*, not a clone of any one of them. A team using
this platform should never need a second tool for "run my IaC," "see what's
deployed where," or "who's allowed to touch this."

## Non-negotiable constraints (carried over from every prior project in
## `/home/soroush/infra/` this operator has built)

- **Air-gap/offline-capable from day one, not bolted on later.** Every
  prior project this session (kubespray-webui, vault-ha, mongodb-cluster)
  had a real offline-install requirement, verified against real target
  images/packages, not just discussed. This platform inherits that bar:
  the control plane itself, and every execution engine's toolchain
  (terraform/ansible/helm/packer binaries, provider/module caches), must
  be installable and runnable with zero outbound internet access once
  staged.
- **Both Docker Compose and Kubernetes are first-class deployment targets**
  for the control plane itself - not "Kubernetes only, Compose is a toy
  demo mode." A single ops team running this for a 20-person org
  shouldn't need to operate a Kubernetes cluster just to run the platform
  that manages *their* Kubernetes clusters.
- **Real verification over assumed correctness.** Every architectural
  claim in this doc set that's checkable against a real system (a real
  Terraform run, a real Vault API call, a real Kubernetes API behavior)
  gets checked against the real thing before being treated as fact, the
  same discipline used throughout this operator's other infra projects
  this session.

## Explicit non-goals (for now)

- Billing/monetization - listed in the original spec as "(future)"; stays
  out of the architecture until a real pricing model exists to design
  against. Speccing billing against a hypothetical pricing model produces
  throwaway design.
- A from-scratch policy language - Sentinel-compatibility and OPA both
  appear in the spec; **OPA/Rego is the actual choice** (open, no license
  encumbrance, genuinely portable), Sentinel compatibility is a
  translation-shim question deferred until a real customer asks for it.
- Building our own Terraform/OpenTofu/Ansible/Helm/Packer *engine* - we
  orchestrate the real upstream binaries (same posture as Atlantis,
  Spacelift, TFC's own execution model), not reimplement HCL evaluation
  or Ansible's module system. This is a scope guardrail, not a detail to
  revisit later: reimplementing any of these is a multi-year effort with
  no product value over correctly orchestrating the real thing.

## How this doc set is organized

Per the requested staged process - each stage is a numbered doc, and each
stage waits for explicit sign-off before the next one starts:

1. `00-vision-and-scope.md` (this doc)
2. `01-architecture-style-and-challenges.md` - monolith-vs-microservices
   decision, language/runtime decision, and a direct challenge to several
   items in the original spec that would hurt the product if built as
   literally stated
3. `02-system-architecture.md` - the actual system diagram, deployment
   topology, and data-plane/control-plane split
4. *(pending stage 1-3 sign-off)* domain model, APIs, database, events,
   per-module detail, UI, backend, workers, integrations, tests,
   deployment
