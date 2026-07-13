# Integrations

Eleventh doc. Deliberately short relative to the module docs - most of
what the original spec calls "integrations" (Git providers, secret
backends, notification channels) already got real architectural
treatment in docs 10, 11, and 14 as **adapters implementing a port a
bounded context already defined** (Stage 10 §3's pattern, applied to
external systems instead of internal cross-context calls). This doc's
actual job is naming the pieces that *don't* already have a home -
Identity Provider auth and Observability wiring - and being explicit
about which spec items need zero new architecture because an existing
pattern already covers them.

## 1. What's already fully specified - not repeated here

- **Git providers** (GitHub/GitLab/Bitbucket/Azure DevOps/Gitea/Forgejo):
  Stage 10 §2-4. Each is an adapter behind a `GitProviderClient` port
  (webhook signature verification, PR-comment posting, check-status
  updates) that GitOps's `/application` layer depends on.
- **Secret backends** (Vault/AWS Secrets Manager/Azure Key Vault/GCP
  Secret Manager): Stage 11 §1. Each is an adapter behind a
  `SecretBackend` port (issue-scoped-token, resolve-value) that the
  Worker plugin process (Stage 9 §5) calls through - **this is also
  where "cloud provider integration" for credentials lives**; the
  platform itself never holds standing cloud credentials for anything
  beyond reaching a secret backend, real Terraform/Ansible cloud auth
  flows entirely through whatever the module/playbook's own provider
  config expects, sourced as ordinary Variables/Secrets (Stage 8) like
  any other input - there is no separate "AWS integration" to design,
  it's the same SecretMount/Variable mechanism already built.
- **Notification channels** (Slack/Mattermost/Teams/Email/Webhook):
  Stage 14 §2. Each is an adapter behind a `ChannelSender` port. **Directly
  generalizes real, already-built code**: this operator's own
  `compose-platform`, built earlier this session, implements exactly
  this shape today (`send_mattermost`, `send_email`,
  `send_syslog_json` functions behind a uniform dispatch call) - the
  Slack/Teams adapters here are the same pattern (all three of
  Slack/Mattermost/Teams accept a simple incoming-webhook JSON POST,
  sharing the bulk of one HTTP-webhook adapter's code with only the
  payload shape differing), not a new design.

## 2. Identity Provider integration - the one genuinely new topic

Three protocols, three well-tested Go libraries, deliberately not
hand-rolled crypto/protocol handling (consistent with Stage 1's
"orchestrate real tooling, don't reinvent it" non-goal, applied here to
auth protocols instead of infrastructure engines):

- **OIDC**: `coreos/go-oidc` (the de facto standard Go OIDC client) -
  standard authorization-code flow, the Control Plane as the OIDC
  Relying Party. Maps the IdP's `sub` claim to `User.external_id`
  (Stage 3 §3), first login with a new `sub` for a given
  `auth_source` provisions a new User (or links to an existing invited-
  by-email User row awaiting first login - a real, common case worth
  naming: an admin invites `alice@corp.com` before Alice ever logs in,
  so User provisioning has to handle "this external_id is new but this
  email already has a pending row" as its own path, not just
  "external_id unseen = always create new").
- **SAML**: `crewjam/saml` (the most maintained Go SAML SP
  implementation) - same `User.external_id` mapping via the SAML
  assertion's NameID.
- **LDAP/Active Directory**: `go-ldap/ldap` - bind-and-search against a
  configured base DN; unlike OIDC/SAML this is the one auth method
  where the Control Plane itself handles a password (proxied straight
  to an LDAP bind, never stored) rather than delegating to a redirect
  flow - worth flagging as a materially different trust shape (the
  platform is in the credential path at all, for this method only)
  from the other two.

**MFA/FIDO2/Passkeys** (spec items): layered on top of **local** auth
specifically (`auth_source: local`) - an org using OIDC/SAML delegates
MFA entirely to their IdP (the standard, correct posture: don't build a
second MFA system that competes with the one the org's real IdP already
enforces) - WebAuthn (`go-webauthn/webauthn`) for FIDO2/passkeys against
local accounts only.

## 3. Observability wiring

OpenTelemetry SDK (Stage 1's principle, now concrete): every `/adapters`
layer (HTTP handlers, gRPC handlers, Postgres repository calls) wrapped
with OTel spans via standard middleware/interceptors, not manually
instrumented call-by-call - a new endpoint gets tracing for free by
being built with the same handler pattern (Stage 10 §2) every other
endpoint uses, not as a separate task per endpoint. Three exporters,
matching Stage 2's observability stack: Prometheus (metrics, pulled via
a `/metrics` endpoint each binary exposes), Loki (structured JSON logs,
shipped via the standard OTel log exporter, not a bespoke log format),
Tempo (traces). **No custom metrics-collection code anywhere in the
application layer** - if it's not automatically captured by the
OTel middleware, it's a gap in the middleware's coverage to fix once,
not a per-feature instrumentation task.

## 4. What this doc explicitly does not add

**Marketplace, Plugin SDK (third-party plugins), Billing, Licensing** -
all real spec items, all explicitly out of scope per Stage 1's
non-goals or genuinely dependent on product decisions (a pricing model,
a plugin distribution/trust model) this architecture phase has no
input to design against yet. Naming them here rather than silently
dropping them, so they're a known, deliberate gap in this doc set, not
an oversight.

## Open questions before the next stage (Tests, then Deployment)

1. **LDAP as a supported auth method for the self-hosted v1 target**:
   confirm it's actually needed for the primary v1 audience (Stage 2) -
   OIDC alone covers a large majority of real deployments (including
   most orgs that technically run AD, via AD's own OIDC/SAML federation
   endpoint rather than raw LDAP), and LDAP's materially different
   trust shape (§2) is worth deferring if nothing in the initial target
   audience genuinely requires raw LDAP bind.
2. **Ready for Tests stage** - given how much of this doc set already
   specified real-infrastructure verification as the working method
   (every module doc's "how would you actually verify this" question
   was implicitly answered by the pattern used throughout this
   session's other projects), confirm proceeding there, understanding
   it may be a short doc that mostly formalizes what's already implied
   rather than introducing much new.
