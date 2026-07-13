# Tests

Twelfth doc, short as flagged at the end of Stage 11 - the *structure*
was already fixed in Stage 10 §6 (domain tests need nothing,
application tests use hand-written fakes, adapter tests need real
infrastructure). What's left is naming the handful of tests that are
specifically load-bearing for correctness claims made earlier in this
doc set, and the one methodological commitment worth stating explicitly
even though it's not new.

## 1. The methodology isn't new - it's what this whole doc set already
## practiced

**Every adapter test runs against a real instance of its
infrastructure** (Postgres, NATS JetStream, MinIO/S3-compatible object
storage - each spun up as an ephemeral container per test run, torn
down after), never a mocked driver. This isn't a fresh policy invented
for this doc - it's exactly the verification discipline this operator
has used for every real decision throughout this actual design
process's own supporting work this session (real CockroachDB containers
to prove migrations, real Vault instances to prove playbook commands,
real Playwright screenshots to prove UI layout fixes) - naming it here
as the codebase's own testing policy, not introducing a new practice
this doc set hasn't already been demonstrating.

## 2. Tests that verify a specific, named claim made earlier in this
## doc set - not generic coverage, targeted at real risk

- **RLS isolation** (Stage 5 §1): a test that creates two Organizations,
  sets `app.current_org_id` to one, and asserts a query for the other's
  data returns zero rows **at the database level** - this is the single
  most security-critical property in the whole system and the test
  exists specifically because "we enforce isolation with RLS" is a
  claim, not a fact, until something actually tries to violate it and
  fails to.
- **Outbox atomicity** (Stage 6 §2): a test that forces a transaction
  rollback *after* the domain write but *before* commit, and asserts no
  outbox row exists - and a separate test that kills the Outbox Relay
  process mid-publish and asserts the event is republished on restart
  (proving at-least-once, not exactly-once, which every consumer's
  idempotency depends on being true, per Stage 6 §5).
- **Stale Run Reaper recovery** (Stage 7 §3): the chaos test this
  pattern specifically needs - start a Run, kill the assigned Worker
  process outright (not a clean shutdown), assert the Workspace lock is
  released and the Run reaches `errored` within the configured timeout
  window, with no manual intervention. Called out because Stage 7
  itself named this "the single most commonly-missed piece in a
  first-pass execution-engine design" - the test exists so it's not
  missed here too.
- **OPA sandboxing** (Stage 9 §4): a policy containing an `http.send`
  call, evaluated through the real restricted builtin set, asserted to
  fail with a builtin-not-found error rather than silently succeeding
  and making the network call - "we disabled `http.send`" is a
  configuration claim until a test actually tries to call it.
- **Secret non-leakage** (Stage 8 §4, Stage 9 §5): a Run configured with
  a known secret value, asserted that the value never appears in the
  Run's stored logs, the `jobs` table, or any outbox event payload -
  generalizing `compose-platform`'s own `log_scrubber` verification
  approach from this session to this platform's equivalent redaction
  path.
- **Plugin protocol contract tests** (Stage 9 §2-3): one shared test
  suite (not one per engine) that runs `Validate`/`Plan`/`Apply`/
  `Destroy` against every real plugin binary in turn, asserting each
  returns the protocol's required message shapes even where an engine's
  own semantics are thin (Compose's best-effort diff, Packer's no-op
  destroy, per Stage 9 §3) - catches "this plugin doesn't actually
  implement the shared interface correctly" as a CI failure, not a
  runtime surprise the first time that engine type is actually used.

## 3. What's explicitly *not* real-infrastructure-tested, and why
## that's fine

Third-party provider/module *behavior* itself (does `aws_s3_bucket`
actually create a bucket correctly) - never this platform's concern to
test, per Stage 1's non-goal of reimplementing engines; this platform's
tests verify it correctly *orchestrates* `terraform apply` and reports
the result, not that Terraform's own providers work, which is
HashiCorp's/the provider maintainer's testing responsibility, not
duplicated here.

## 4. CI shape

- **Every PR**: domain + application tests (fast, no infrastructure) +
  adapter tests against ephemeral containers (§1) + the §2 named tests
  + a real `golang-migrate up` against a fresh Postgres container,
  proving every migration in the PR actually applies cleanly, the same
  check this operator has run manually throughout this session's other
  Alembic-based projects, now a permanent CI gate instead of a manual
  step.
- **Pre-release only** (not every PR - genuinely slower, real-cluster-
  dependent): a full end-to-end Run through every engine type against
  real (disposable, torn-down-after) target infrastructure - this is
  where "does a Kubespray Run actually stand up a working cluster"
  gets proven, not on every commit.

## Open questions before the final stage (Deployment)

1. **Pre-release e2e target infrastructure**: real cloud accounts
   (cost, and conflicts with the air-gap-first framing if the *only*
   verification path needs live cloud access) vs. a fully local target
   (LocalStack for cloud-API-shaped tests, real local VMs/containers for
   Ansible/Kubespray targets) - given Stage 1's air-gap constraint,
   this doc leans toward "the pre-release suite should be runnable
   without real cloud credentials, even if a supplementary real-cloud
   suite exists separately for provider-specific confidence" - confirm.
2. **Ready for the final stage: Deployment** - the Helm chart and
   docker-compose.yml this entire design compiles down to, closing the
   loop back to Stage 2's two topologies with everything decided since
   (the object storage layout, the Runnable supervision model, the
   Worker container-isolation requirement) now concrete enough to
   actually specify real deployment manifests against.
