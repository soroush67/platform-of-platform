# Backend structure

Tenth doc - the actual Go package layout implementing docs 03-17. This
is where Stage 1's "hexagonal architecture, DDD, no module reaching into
another's implementation details" commitment stops being a principle
and becomes a concrete rule about which package is allowed to import
which.

## 1. Repository layout

```
/cmd
  /control-plane        main() - wires everything in §5, starts the HTTP/gRPC
                         servers and the background Runnables (§4)
  /worker                main() - the Stage 9 Worker process
  /cli                    main() - thin wrapper generated against the
                         OpenAPI spec (Stage 4 §9), plus command-palette-
                         style ergonomics on top
/internal
  /platform               cross-cutting, no domain knowledge: config
                         (env-var loading, Twelve-Factor), logging,
                         OpenTelemetry wiring, a shared error type
                         (mapping domain errors to Stage 4 §7's RFC 7807
                         shape at the HTTP boundary)
  /outbox                 the Transactional Outbox mechanism (Stage 6
                         §2) - shared infrastructure every context's
                         adapters use, not itself a bounded context
  /tenancy, /identity, /rbac, /workspace, /execution, /variables,
  /secrets, /state, /policy, /gitops, /approval, /kubernetes,
  /registry, /audit, /notifications
                         one directory per Stage 3 bounded context,
                         internal structure per §2
/pkg
  /apiclient              the generated Go SDK (Stage 4 §9) - the one
                         thing under /pkg because /cmd/cli and any
                         future Go integration genuinely need to import
                         it from outside this module's own binaries
```

`internal/` (Go's own compiler-enforced "nothing outside this module
can import this" boundary) for everything else - not a convention
this doc has to police by review, the Go toolchain itself refuses a
build that violates it.

## 2. Per-context internal structure - the concrete hexagon

Using Execution (the most detailed module doc) as the worked example;
every other context under `/internal` follows the identical shape:

```
/internal/execution
  /domain                 pure Go, zero imports outside the Go stdlib
                         and other contexts' /domain packages (see §3
                         on why even that's restricted) - Run, Job
                         (Stage 3 §6), the state machine transition
                         rules (Stage 7 §1) as methods on Run
                         (`func (r *Run) Apply() error` returns an
                         error if called from a non-apply-eligible
                         status, so an invalid transition is a
                         compile-reachable bug caught by a unit test
                         against *only* this package, no database, no
                         HTTP, nothing to mock)
  /application             orchestrates domain objects against *ports*
                         (Go interfaces this package declares, not
                         imports from elsewhere - see §3):
      ports.go             type RunRepository interface { ... }
                            type WorkspaceLocker interface { ... }  // Stage 5 SS2's SELECT...FOR UPDATE, behind an interface
                            type VariableResolver interface { ... } // Stage 8 SS3's shared resolution function, called through this port
                            type WorkerDispatcher interface { ... } // Stage 4 SS10's JobAssignment send
      trigger_run.go        the `POST /runs` use case (Stage 7 SS2):
                            validate → RunRepository.Save → outbox event
      dispatcher.go         the Run Dispatcher Runnable (SS4)
      stale_reaper.go        the Stale Run Reaper Runnable (SS4)
  /adapters
    /postgres               implements RunRepository (Stage 5's `runs`/
                            `jobs` tables), WorkspaceLocker
    /http                   Stage 7 SS2's REST handlers - parse
                            request → call an /application use case →
                            map result to response DTO. No business
                            logic lives here, ever - a handler that
                            contains an if-statement deciding whether a
                            Run can be canceled is a handler that put
                            domain logic in the wrong layer.
    /grpc                   the WorkerDispatch server (Stage 4 SS10) -
                            same rule, translates wire messages to
                            /application calls, no logic of its own
    /workspace_adapter      implements VariableResolver by calling
                            into the workspace context's own public
                            application-layer function (SS3) - this is
                            the one adapter in this list that doesn't
                            talk to external infrastructure, it
                            satisfies a port with another context
```

## 3. Cross-context calls - dependency inversion, not direct imports,
## and never into another context's `/domain` or `/adapters`

Stage 1's "no module reaching into another's implementation details"
becomes a concrete rule: **a context may only depend on another
context's `/application` package's exported functions (its "public
API" within the monolith), never its `/domain` types directly and never
its `/adapters` at all.** When Execution's Run Dispatcher needs
Workspace's variable resolution (Stage 8 §3), it doesn't import
`internal/workspace/domain` - it declares a `VariableResolver` port in
its *own* `/application/ports.go` (the type Execution actually needs,
shaped for Execution's own use, not Workspace's internal representation)
and `internal/workspace` provides the adapter that satisfies it. This
is dependency inversion applied deliberately, not just a hexagonal-
architecture formality: it means Workspace's internal domain model can
change shape without Execution's code changing at all, as long as the
adapter still satisfies the port Execution defined - the same property
that makes swapping Postgres for a different database only touch
`/adapters/postgres`, applied to *inter-context* dependencies instead of
just *infrastructure* dependencies.

**Events (Stage 6) remain the mechanism for reactions, this section is
only about the handful of genuinely synchronous cross-context reads**
(variable resolution needing an answer *now*, in the same request, is
the concrete case; "Notifications reacts to Execution completing a Run"
is not this pattern, it's the outbox/event path from Stage 6, and
Notifications never gets a synchronous port into Execution at all).

## 4. Background Runnables - one supervision pattern for every
## scheduler this doc set has accumulated

Every background loop from docs 06-17 (Outbox Relay, Run Dispatcher,
Stale Run Reaper, Scheduled Run Trigger, Cluster Sync scheduler, Module
Version Ingestion trigger) implements one shared interface:

```go
type Runnable interface {
    Run(ctx context.Context) error   // blocks until ctx is canceled or a fatal error
}
```

`cmd/control-plane/main.go` starts every registered `Runnable` in its
own goroutine under one `errgroup.Group` tied to a context that's
canceled on `SIGTERM` - standard Go graceful-shutdown, chosen
specifically because it's one supervision mechanism for six
structurally similar "poll/react on an interval or a queue" loops
rather than six bespoke goroutine-management implementations, and it's
what makes the Twelve-Factor "disposability" property (Stage 1) an
actual property of the binary rather than an aspiration - a `SIGTERM`
during an in-flight Outbox Relay batch or Run Dispatcher claim lets
that unit of work finish or cleanly abort, not get killed mid-write.

## 5. Dependency injection - manual, explicit, wired once in `main()`

**Deliberately no DI framework** (`google/wire`, `uber/fx`, or similar) -
every port from every context's `ports.go` gets its concrete adapter
constructed and passed in explicitly, in one place
(`cmd/control-plane/main.go`), in dependency order. More lines of
wiring code than a framework would need, on purpose: **every
dependency this system has is greppable and debuggable by reading one
file**, rather than a framework resolving a dependency graph via
struct tags or code generation a new engineer has to learn the
framework's own conventions to trace. This is the same "avoid
unnecessary abstraction, three similar lines beat a premature
abstraction" principle already applied throughout this doc set, applied
here to DI specifically because it's the place Go projects most often
reach for a framework out of habit rather than genuine need - at the
scale of ~16 contexts wired once at startup, explicit wiring hasn't
earned its complexity cost yet, and if it ever grows unwieldy enough to
justify one, `wire`'s compile-time code generation (not `fx`'s runtime
reflection) would be the fallback, since it generates the same kind of
explicit code this section already produces by hand rather than hiding
the graph behind runtime magic.

## 6. Where tests live (structure only - testing *strategy* is Stage 0's
## own later "Then tests" phase, not duplicated here)

`_test.go` files colocated with the code per Go convention:
`/domain` tests need nothing but the package itself (Stage 7 §1's state
machine is exactly the kind of logic this makes trivial to test
exhaustively - every transition, every invalid-transition rejection, no
database or mock required); `/application` tests use hand-written fakes
implementing the `ports.go` interfaces (not a mocking framework - a
fake `RunRepository` backed by an in-memory map is both simpler to
write and less brittle than a generated mock asserting exact call
sequences); `/adapters/postgres` tests run against a real ephemeral
Postgres (the same `docker run cockroachdb/cockroach` -style real-
infrastructure-in-a-container verification this operator has used
throughout this session's other projects, not a SQL-mocking library
that can't actually catch a bad migration or an RLS policy that isn't
doing what it's supposed to).

## Open questions before the next stage (Workers implementation detail,
## Integrations, or Deployment)

1. **Confirm the `/application`-only cross-context boundary (§3)** is
   the right strictness level - it's stricter than many Go monoliths
   enforce in practice (most just rely on convention, not a hard rule),
   chosen deliberately given Stage 1's explicit "no module reaching
   into another's implementation details" requirement, but worth a
   conscious yes given it does mean more `ports.go` interfaces than a
   looser convention would need.
2. **Which stage next?** The Backend structure here is generic across
   every context; Workers (Stage 9) already got its own protocol/
   isolation doc, so the remaining Stage 0 items are Integrations (the
   actual GitHub/GitLab/Slack/cloud-provider API client code, mostly
   mechanical against docs already written), Tests (strategy, coverage
   targets, the real end-to-end verification approach - given how much
   of this doc set already leaned on "verify against real
   infrastructure" as a working method, this might be short), and
   Deployment (the actual Helm chart / docker-compose.yml this whole
   design compiles down to, closing the loop back to Stage 2's two
   topologies). Deployment is arguably the most natural closing doc
   given it's where every prior decision becomes something runnable -
   your call on order.
