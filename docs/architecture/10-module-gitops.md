# Module detail: GitOps / Git Integration

Fourth module doc - closes the loop: everything so far specified how a
Run progresses once it exists, this is where most Runs actually come
from in real usage (a `terraform apply` triggered by hand is the
exception in a mature GitOps workflow, not the common case).

**Resolves Stage 9's open question**: policy evaluation failures
(distinct from *finding* a violation) fail closed for
`hard_mandatory`/`soft_mandatory` PolicySets, fail open (proceed with a
logged warning) only for `advisory` ones - an unevaluable mandatory
policy is treated as "we don't know if this is safe," which defaults to
blocking, the same posture a CI pipeline takes when a required check
errors rather than passes or fails.

## 1. Two aggregates, two different lifecycles

- **GitConnection** (org-scoped): the *credential and provider
  relationship* - "this org can talk to this GitHub org/GitLab group as
  this identity." Created once per provider account, reused across many
  workspaces.
- **RepositoryLink** (workspace-scoped): *which repo+path+branch this
  specific workspace tracks*, and what happens when it changes. Many
  RepositoryLinks can share one GitConnection (a monorepo with 10
  workspaces, one path each, all under one GitHub App installation).

## 2. REST API - GitConnection

`/api/v1/orgs/{org}/git-connections`

```json
// POST
{
  "provider": "github",                 // github | gitlab | bitbucket | azure_devops | gitea | forgejo
  "auth_method": "deploy_key",          // deploy_key | oauth_app | pat
  "credential": "-----BEGIN OPENSSH PRIVATE KEY-----..."   // write-only, stored via Secrets context (Stage 3 §8), never returned by GET
}
```

**Auth method trade-off, worth stating since the spec lists all three
without ranking them**: `deploy_key` is the recommended default -
repo-scoped, no standing access to anything else in the provider org,
smallest blast radius if it leaks. `oauth_app` (a GitHub App
installation, GitLab's equivalent) is for org-wide setups where
provisioning a separate deploy key per repo doesn't scale, and comes
with its own fine-grained permission model at the provider side. `pat`
(personal access token) is supported because it's the fastest path to a
working demo, but the Web UI surfaces a persistent warning on any
GitConnection using one - it's tied to a human's account and breaks the
moment that person leaves or rotates their token, which is a real,
recurring operational failure mode this doc wants flagged rather than
silently accepted as equally good.

## 3. REST API - RepositoryLink

`/api/v1/orgs/{org}/projects/{project}/workspaces/{workspace}/repository-link`
(singular path - Stage 3 §11 already established one link per
workspace)

```json
{
  "git_connection_id": "uuid",
  "repo": "my-org/infra",
  "path": "environments/prod/vpc",       // subdirectory, for monorepo support
  "trigger_mode": "auto_plan_only",      // manual | auto_plan_only | auto_plan_and_apply | tag_push
  "trigger_ref": "main",                 // branch name, or a glob for trigger_mode=tag_push (e.g. "v*")
  "post_pr_comments": true               // only meaningful for auto_plan_only
}
```

## 4. Incoming webhook handling (fills in Stage 4 §8's shape)

```
POST /api/v1/webhooks/{provider}
  1. Verify signature against the matching GitConnection's stored secret
     (HMAC for GitHub/GitLab, provider-specific scheme otherwise) -
     reject with 401 before any parsing if this fails, so an
     unauthenticated payload never reaches step 2's business logic.
  2. Parse into a normalized internal shape:
     { repo, ref, ref_type: branch|tag, event: push|pr_opened|pr_updated,
       pr_number (nullable), head_sha }
  3. Find every RepositoryLink where repo matches and trigger_ref
     matches ref (exact for branch, glob for tag_push)
  4. For each match, based on trigger_mode:
     - manual:                only update workspace.latest_known_commit, no Run
     - auto_plan_only:        POST /runs equivalent, target=plan_only
     - auto_plan_and_apply:   POST /runs equivalent, target=plan_and_apply
     - tag_push:               same as auto_plan_and_apply, gated on ref_type=tag
  5. Emit `gitops.webhook_received` (Stage 6) regardless of whether a
     Run was created - Audit gets a record of every webhook received,
     not only the ones that triggered something, which matters for
     debugging "why didn't my push trigger a plan" support cases.
```

## 5. PR/MR comment integration (auto_plan_only's actual payoff)

A background consumer of `run.plan_completed` (Stage 6), filtered to
Runs where `trigger = vcs_pr` and the owning RepositoryLink has
`post_pr_comments: true`: posts a comment on the source PR/MR via the
provider's API, containing a plan summary (resource
add/change/destroy counts) and a link back into the platform's Web UI
for the full plan output. This is the single feature that makes
`auto_plan_only` actually useful day-to-day (a reviewer sees the
infrastructure impact of a PR without leaving the code review) rather
than just a Run trigger-mode variant with no reason to prefer it over
checking the Web UI directly.

## 6. Branch protection / required status checks

Not something this platform manages *inside* the provider (GitHub's own
branch protection rules stay GitHub's) - the platform's contribution is
**exposing a Run's status as the provider's native "check" API**
(GitHub Checks API, GitLab's commit status API) as soon as a Run tied to
`trigger: vcs_pr` starts, updating it through `planned`/`policy_failed`/
etc. This is what lets an org configure "require the infra plan check to
pass before merge" using the provider's own branch-protection UI they
already know, instead of the platform reinventing merge-gating.

## Open questions before the next module doc

1. **`tag_push` semantics**: this doc assumed tag pushes always mean
   `plan_and_apply` (a tag is usually a deliberate "ship this" signal) -
   confirm, or should tag-triggered runs also default through the
   normal approval gate rather than being treated as pre-approved by
   the act of tagging?
2. **Which module next?** Secrets & State Management are the two
   contexts every executed Run actually touches but neither has been
   detailed yet beyond Stage 3's aggregate sketch and Stage 8 §4's
   "Worker resolves it" boundary statement - recommend those two
   together next (they're both "how does a Job actually get the inputs
   it needs and record what it produced," a natural pair), then
   Kubernetes and Registry after, leaving RBAC/Identity/Tenancy's own
   full CRUD API (already load-bearing everywhere via Stage 3-4's
   design, but never given its own endpoint-by-endpoint doc) and
   Notifications/Audit's own APIs (mostly read/query surfaces at this
   point, the write side was fully specified in Stage 6) for last,
   since they're the lowest-risk/most-mechanical remaining docs.
