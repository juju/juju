---
myst:
  html_meta:
    description: "Juju's architecture from first principles: intent and execution,
      the watcher/worker loop, the execution chain, and the design choices that make
      the whole system eventually consistent and self-healing."
---

(architecture-principles)=
# Juju architecture: principles

This document explains *why* Juju is built the way it is, starting from the
problem and letting the architecture fall out. It is a complement to
{ref}`juju-architecture`, which catalogs the data model and operations in
detail. Read this one first if you want the mental model before the map.

---

## Layer 1 -- The problem and the three separations

You want to operate applications on cloud infrastructure: install, configure,
integrate, scale, upgrade, remove. You want this to work on any cloud, without
writing cloud-specific or application-specific glue code every time, and you
want it to keep working through restarts, failures, and drift.

Three requirements follow immediately.

**Separate intent from execution.** The person deciding *what should be* cannot
also be responsible for knowing *how to make it so* on every cloud and for every
application. You need a declaration of the desired state, and a mechanism that
knows how to realize it.

**Separate cloud logic from application logic.** Provisioning a machine from
AWS and operating a PostgreSQL database are fundamentally different kinds of
knowledge. If they are entangled -- if the database operator must know AWS, or
the AWS provisioner must know PostgreSQL -- neither is reusable and both are
fragile. They must be separate concerns.

**Be distributed and agent-driven.** Intent and execution are separate, and they
must be kept in agreement across many hosts, over a long time, as machines
disappear and demands change. The structure that does this at scale is a central
place that holds the intent, and a set of agents -- one beside each workload —
that watch the intent and drive the real world toward it.

These three requirements have a natural conclusion. Their realization is:

- A **controller** that owns the declared state.
- **Charms** that own the application knowledge.
- **Cloud providers** that own the infrastructure knowledge.
- **Agents** that close the gap between declared state and the real world,
  running on every provisioned host.

<!-- ILLUSTRATION NEED: "The three separations" overview diagram.
     What to show: user → controller (intent/persistence layer), controller ↔
     cloud (infrastructure knowledge), controller ↔ Charmhub (application
     knowledge), controller → agents (execution). Horizontal axis: intent to
     execution. Vertical: cloud above, Charmhub below. This is the "insight"
     diagram from architecture.md but annotated with the three separation labels.
     ggarch: existing "Juju overview" view in juju.ggarch is close; add
     annotation regions for the three separations. -->

---

## Layer 2 -- Intent, persistence, execution

Given the three separations, the next question is: what does the controller
actually hold, and how does it drive agents?

### Intent: the controller DB is goal state

When you run `juju deploy postgresql`, nothing happens to any machine. The
controller writes a record to its database. That record is the **goal state**:
"there should be a PostgreSQL application with N units". The database is the
single source of truth. It is strongly consistent -- replicated across HA
controller nodes via Raft, ACID-transactional, durable across restarts and
network partitions.

Every operation you perform as a user -- deploy, integrate, scale, upgrade,
remove -- is a write to the controller database. The declaration of intent is
always complete before any execution begins.

### Persistence: the controller DB as the substrate

The controller holds two kinds of database:

- The **controller database** records the infrastructure: clouds, credentials,
  users, models, SSH keys, secret backends. This is the shared plumbing that
  any controller has, regardless of what is deployed.
- One **model database** per model records the deployment: applications, charms,
  units, machines, relations, storage, secrets. This is the self-contained world
  of that model.

The split reflects the architecture: one controller, many models. The model
database is the model's entire world -- which is why model migration is possible
as a relatively self-contained operation.

### Execution: agents close the gap

Agents are the execution side. Every provisioned host runs an agent process
(`jujud` on machine clouds, `containeragent` on Kubernetes). The agent's job is
to watch the controller's declaration of what should be, compare it against
what actually is, and act to close the gap.

The connection is **one-directional in terms of data**: the agent connects to
the controller, never the other way around. The controller never pushes state to
agents -- it only fires **notifications** when something changes. The agent pulls
the relevant state on its own initiative.

This is not a subtle point. It is a fundamental design choice with two
consequences:

1. **The controller is passive with respect to agents.** It holds state and
   serves watchers. It never reaches out. An agent that is partitioned, crashed,
   or slow does not break the controller or any other agent -- it just falls
   behind.
2. **The system is AP** in the CAP theorem sense. The controller database is
   strongly consistent (CP at rest), but the distribution of state changes to
   agents is eventually consistent. Agents converge toward declared state; they
   are not guaranteed to converge simultaneously. This is a deliberate tradeoff:
   it makes the system resilient to network segmentation, at the cost of
   temporary inconsistency between goal state and observed reality.

<!-- ILLUSTRATION NEED: "Intent, persistence, execution" -- three horizontal
     bands. Top: user writes intent (client → controller API). Middle:
     controller DB stores goal state (persistence). Bottom: agents reconcile
     (agent ← watcher fires → agent re-reads → agent acts). Arrows down from
     intent to persistence, down from persistence to execution, and a feedback
     arrow from execution back up to persistence (agent declares its own state).
     ggarch: new view or annotations on existing topology. The key visual
     element is that the notification arrow is a thin async signal (EventType),
     while the data-fetch arrow is a fat sync pull (API call). These need
     visually distinct edge types -- ggarch "event" vs "api" types fit perfectly.
-->

---

## Layer 3 -- The watcher and worker loop

How does an agent actually know when something changed? How does it stay alive
and healthy? This layer is the machinery behind the arrows.

### Watchers: signals, not data

Every agent holds **watcher** connections to the controller API. A watcher is a
Go channel that fires when something relevant to this agent changes in the
controller database. The key property -- and the most important thing to
understand about the Juju architecture -- is: **a watcher delivers a signal, not
data.**

There are two kinds of watcher in practice:

- **NotifyWatcher** -- fires an empty `struct{}`. Pure signal. "Something
  changed. Go find out what."
- **StringsWatcher** -- fires a slice of the *keys* (IDs) of records that
  changed. Still no data -- just enough to know which records to re-read.

When a watcher fires, the agent does not have the new state in its hand. It
must call the API to fetch it. This is deliberate. The change-stream records
only that something changed -- not what it changed to -- because by the time the
notification arrives, the original change may be stale (further changes may
have happened). Rather than propagate potentially-stale data, the system
propagates only the notification, and the consumer always fetches the current
authoritative state.

```
internal/changestream/stream/doc.go:
  "This information all amounts to a notification that something has happened.
   The reason no specifics about what exactly has happened are included is
   because, as in every eventually consistent system, that information can
   easily get stale. To retrieve the latest information, each subscriber must
   query the database when they receive the notification."
```

A second property: every watcher is **required** to fire once immediately on
creation, delivering the current baseline state before any deltas. This
"at-least-one notification" guarantee is the startup sync: a freshly-started
agent does not need a special initialization path distinct from the reconcile
loop -- it just receives its initial event and reconciles from there.

### Workers: the standard loop

An agent is not a single monolithic process -- it is a **tree of workers**, one
per concern. Each worker is a Go goroutine (or small group of goroutines) that:

1. Creates one or more watchers and registers them.
2. Waits on `watcher.Changes()` or a shutdown signal.
3. On a notification: fetches current state, diffs against local knowledge,
   acts to reconcile, declares any updated state back to the controller.
4. On any error: exits immediately. Does not retry. Does not try to recover.
   That is someone else's job.

The standard loop is:

```go
for {
    select {
    case <-catacomb.Dying():
        return catacomb.ErrDying()   // graceful shutdown
    case <-watcher.Changes():
        state := api.FetchCurrentState()
        if state != localState {
            act(state)
            localState = state
        }
    }
}
```

The "exit on error" rule is not carelessness -- it is a correctness property.
A worker that catches its own errors and retries is a worker that may continue
operating on an inconsistent local view of the world. The system is built around
restarts, not recovery.

### Tomb and catacomb: supervised lifetimes

Two libraries manage worker lifetime:

- **Tomb** (`gopkg.in/tomb.v2`) manages the lifecycle of a single goroutine:
  `Kill(err)` to stop it, `Wait()` to wait for it to finish, `Dying()` channel
  to detect shutdown from inside the loop. A tomb is for leaf workers with no
  children.
- **Catacomb** (`github.com/juju/worker/v5/catacomb`) adds **child supervision**
  on top of a tomb. A worker that owns child workers and watchers uses a
  catacomb: `catacomb.Add(childWorker)` registers a child so that if the child
  dies with an error, the catacomb (and its parent worker) dies with that error.
  The catacomb also provides a `Context` canceled on shutdown, so API calls
  bounded to the worker's lifetime will not hang after the worker is killed.

The rule: **leaf worker uses tomb; any worker with children uses catacomb.**

### The dependency engine: the worker tree

Workers are assembled into trees using the **dependency engine**
(`github.com/juju/worker/v5/dependency`). Each worker is a **manifold** -- a
declaration of:

- `Inputs`: named other workers this one depends on.
- `Start`: a function that receives the outputs of those dependencies and
  constructs the worker.

The engine starts manifolds in dependency order, and supervises them: if a
worker exits, the engine restarts it (with backoff); if it exits too many times,
the engine stops its dependents too. A failure in one branch of the tree
propagates to everything that depends on it -- the tree collapses toward the
failed node, not away from it. This is the "prefer restart over running broken"
principle: the system would rather go through a clean restart than continue
operating with a subtree in an unknown state.

<!-- ILLUSTRATION NEED: Worker tree / dependency engine.
     What to show: an agent's manifold graph. Roughly: api-caller at root,
     several workers hanging off it (deployer, uniter, leadership, watchers).
     A failure in one node killing its subtree. Possibly two separate panels:
     healthy tree and post-failure collapse.
     ggarch: state machine view would work for the lifecycle (alive → dying →
     dead). A topology view works for the tree structure. The "lifecycle:
     ephemeral" attribute on workers that are transient is meaningful here.
     Two illustrations: (1) topology of manifold dependencies; (2) state
     machine of a worker's lifecycle. -->

---

## Layer 4 -- The execution chain

Now zoom in on what actually happens when a unit runs. The path from "controller
decided a machine should exist" to "charm code ran" passes through five distinct
stages, each supervised by its own worker.

### Stage 1: The compute provisioner

The **compute provisioner** is a controller-side model worker. It watches
`WatchModelMachines` -- a `StringsWatcher` of machine IDs in the model. When a
new machine record appears (because a deploy or scale-up wrote one), the
provisioner fetches the provisioning parameters -- agent config, tools, API
addresses, placement constraints, availability-zone preferences -- and calls
`Broker.StartInstance` on the cloud. When the cloud responds, the provisioner
records the instance ID back to the controller (`SetMachineCloudInstance`).

The provisioner must run on exactly one controller node in an HA deployment. It
achieves this through the dependency graph: its `Environ` input resolves only on
the responsible controller, so the manifold never starts on the others.

### Stage 2: The machine agent

The provisioned machine boots with the Juju agent binary installed. The
**machine agent** (`jujud machine`) starts, connects to the controller API, and
runs its own dependency engine. Its manifold set includes:

- Upgrade gating (the machine waits for upgrade to complete before running
  workers that depend on the current version).
- A **deployer** worker that watches which units are assigned to this machine and
  manages their lifetimes.
- Workers for storage, LXD container provisioning, networking, log forwarding,
  and machine health reporting.

### Stage 3: The unit agent (in-process)

On machine clouds the unit agent is not a separate process. The deployer worker
creates a **nested dependency engine per unit**, running inside the machine
agent's process. This per-unit engine is the "unit agent". Its primary worker is
the **uniter**.

On Kubernetes, the unit agent is a separate container process (`containeragent`),
but the logic is identical.

### Stage 4: The uniter resolver loop

The **uniter** is the worker that makes a unit converge toward its declared
state. It has two moving parts:

**The remote-state watcher** fans several independent watchers -- unit lifecycle,
application config, relations, storage, secrets, leadership, workload events —
into a single coalescing `Snapshot`. All individual watcher changes collapse into
one broadcast per "batch of changes", so the resolver loop always works from a
coherent snapshot of remote state.

**The resolver loop** (`resolver.Loop`) is the convergence engine:

```
snapshot ← RemoteState.Snapshot()
op ← Resolver.NextOp(localState, snapshot, factory)
Executor.Run(op)       // prepare → execute → commit
refresh snapshot + local state
repeat
```

`NextOp` inspects the diff between what the unit's local state says has
happened and what the remote snapshot says should be happening, and decides the
next operation: a hook to run, an upgrade to perform, an action to execute. If
nothing needs doing, it returns `ErrNoOperation` and the loop idles.

`Executor.Run` is a three-phase state machine -- **prepare → execute → commit**
— with the intermediate state written to durable storage after each phase. This
means a crash between phases lands the unit in a known partial state that the
resolver can recover from deterministically on restart.

An operation that fails at the execute phase (hook failure) leaves the unit in
`error` state. The Executor commits the error state; the resolver's next
`NextOp` sees the unit in error and returns `ErrWaiting` until a user
intervention (`juju resolved`) clears it.

### Stage 5: Charm execution and the hook command server

When the resolver decides a hook must run, it calls `runner.RunHook(name)`:

1. **The jujuc server starts.** The runner opens a Unix domain socket and starts
   an in-process RPC server (`jujuc.Server`). This server is bound to the
   current hook's execution context -- it knows the unit, the relation state, the
   secret backends, the leadership status.
2. **Hook vars are set.** The runner builds the hook's environment variables:
   `JUJU_CONTEXT_ID`, `JUJU_AGENT_SOCKET_ADDRESS`, `JUJU_DISPATCH_PATH` (the
   hook path, e.g. `hooks/config-changed`), `JUJU_CHARM_DIR`, and many others.
3. **The charm process is exec'd.** The runner discovers whether the charm uses
   the operator-framework dispatch mechanism (a single `dispatch` script) or
   per-hook scripts, and forks the appropriate process.
4. **Hook tools are symlinks.** Every `juju-*` hook tool (`config-get`,
   `relation-get`, `status-set`, etc.) is a symlink to the `jujuc` binary. When
   the charm calls `config-get`, jujuc reads `JUJU_AGENT_SOCKET_ADDRESS`, dials
   the Unix socket, and sends an RPC request to the jujuc server running in the
   uniter. The server executes the command against the hook context and returns
   the result. **No hook tool ever goes to the controller directly -- they all go
   through the in-process server.**
5. **Commit or discard.** On clean exit (exit code 0): the operation's Commit
   phase flushes buffered writes (relation data changes, status updates) to the
   controller API. On failure: the Executor records the error state; no writes
   are flushed. The hook's effects are atomic with respect to the unit's declared
   state.

<!-- ILLUSTRATION NEED: End-to-end execution chain (vertical or left-to-right).
     Five stages stacked or chained: (1) controller provisioner watches
     machines DB, calls cloud StartInstance; (2) machine agent starts on
     provisioned host, runs deployer; (3) per-unit engine (nested) starts
     uniter; (4) uniter resolver loop: snapshot → resolve → execute → commit;
     (5) charm hook: exec dispatch, hook tools → jujuc server → controller API.
     Key visual: each stage is a worker loop, and the output of each stage is
     the input of the next. The edge type from provisioner to machine is
     "cloud instance created" (not a Juju API call); from machine agent to unit
     agent is a local in-process call; from uniter to charm is exec(); from
     charm to jujuc server is a Unix socket RPC.
     ggarch: this could be a sequence diagram across the five participants, or a
     topology diagram with the execution chain as a horizontal flow. The
     lifecycle attributes (init: for the provisioner's one-shot nature,
     ephemeral: for hook execution) would be meaningful. The edge type variety
     here (control, ipc, api, cloud StartInstance) is a good test of ggarch's
     typed-edge expressiveness. -->

---

## Layer 5 -- Consistency and relations

### Eventual consistency and AP

The system as a whole is **AP**: available and partition-tolerant, with eventual
consistency. The controller database is strongly consistent (Raft, ACID). The
distribution of state to agents is eventually consistent.

Concretely: if the controller's declaration of goal state and the real world
diverge -- because an agent is partitioned, a machine reboots, a hook fails —
the system does not treat this as a fatal error. The agent, when it reconnects,
will receive its initial watcher event (the at-least-once guarantee), snapshot
the current state, and resume converging. The only invariant is *eventual*
convergence, not simultaneous convergence.

This makes the system resilient to segmentation. A model spread across a
flaky network can still make progress: agents reconcile independently, and
the controller is the meeting point they converge toward.

The corollary is that **state is never propagated, only declared.** Every
component in the system follows the same pattern:

- Something changes (a user action, a watcher fires, a hook completes).
- The component re-reads the authoritative state from the controller.
- The component acts to close the gap.
- The component **declares** its own new state back to the controller via API
  calls -- it does not push state to peers.

No component ever pushes state directly to another component. There are no
peer-to-peer channels. The controller is the single shared medium.

### The star topology of relations

This "no peer communication" principle has a visible architectural consequence:
**integrations form a star topology**.

When two applications integrate, their unit agents never communicate directly.
The protocol is:

1. Unit A writes its relation data bag to the controller (`relation-set` →
   jujuc server → `UniterAPI.SetRelationUnitSettings` → controller DB).
2. The controller fires a watcher notification to unit B.
3. Unit B's watcher fires; it calls `relation-get` → jujuc server →
   `UniterAPI.ReadSettings` → reads A's data bag from the controller DB.
4. Unit B decides its next relation hook, runs it, and writes its own data bag
   back to the controller.
5. The controller notifies A, and A reads B's data bag.

The controller is always in the middle. Unit A and unit B are never connected.
This is not just an implementation detail -- it is a key architectural property.
Every relation, every integration, passes through the controller. Relation data
lives in the controller database. The controller is the meeting point for every
application-to-application agreement.

The star topology has costs (all relation data travels through the controller)
and benefits (the controller is the single auditable source of truth for every
integration state; cross-model relations and cross-controller integrations work
because both sides speak only to their own controller).

<!-- ILLUSTRATION NEED: Star topology of relations.
     What to show: two (or three) unit agents, each connected to the controller.
     Unit A writes data to controller; controller notifies B; B reads from
     controller; B writes back; controller notifies A. No A→B edge.
     Emphasis on: relation data lives in controller DB, not in either agent.
     ggarch: this is a natural topology + annotation combination. The controller
     sits in the middle. Annotation regions: "relation data bag (app A)" and
     "relation data bag (app B)" inside the controller, not inside the unit
     pods. Edge types: "data" for writes, "event" for watcher notifications,
     "api" for reads. This diagram would require ggarch to support small
     "sub-node" labels or record-style nodes for the data bags -- a good
     expressive power test.
     Variant: show three integrated applications to make the star shape obvious
     (A-B-C all through controller, no direct A-C edge). -->

### No exception: everything declares state

The unifier of all the above is one rule with no exceptions:

> **Every component in Juju declares its state to the controller. Nothing
> propagates state to peers. Everything is woken up by a watcher notification
> and re-reads authoritative state before acting.**

This applies to:
- Clients (`juju deploy` writes goal state to the DB).
- The provisioner (writes machine instance IDs to the DB).
- Unit agents (write hook completion, relation data, status to the DB).
- Charms (write relation data and status via hook commands → unit agent → DB).
- The machine agent (writes agent version, machine health to the DB).
- Even the controller's own charm (records its state to the same DB as any
  other charm would, though it is not operated via hooks).

The result is a system where the controller database is the complete, auditable,
single source of truth for the entire deployment at all times. Any component can
be restarted from scratch, connect to the controller, receive the initial watcher
event, and resume from a coherent state without any peer needing to re-send
anything.

---

## Layer 6 -- Failure handling and the restart preference

### A failure in one branch collapses the tree

The dependency engine's failure propagation is intentional: a failing worker
kills its dependents, not just itself. This is not a bug or an oversight -- it
reflects a clear design preference.

**Prefer clean restart over running broken.** A worker that has failed may have
left its local state inconsistent. Its dependents may have been operating on
assumptions that no longer hold. Rather than attempt to patch up a partially
broken subtree, the engine collapses the affected branch and restarts it from
scratch. The restart inherits the same watcher-driven convergence: the fresh
worker receives its initial watcher event and derives the correct state from
what the controller currently says.

This is an expression of eventual consistency at the process level, not just
the network level. A unit agent that crashes and restarts is not a special case
— it is the normal operating mode. The system is designed to tolerate it
without operator intervention.

### Tombs, catacombs, and the ErrDying sentinel

The failure model is precise down to the library level:

- `tomb.ErrDying` is the graceful shutdown sentinel -- a worker returns it when
  it exits because it was asked to stop, not because something went wrong. The
  tomb records this as a nil error, not an error.
- `catacomb.ErrDying` is the same sentinel for catacomb-managed workers.
- Neither of these sentinels should ever escape a worker boundary. If unit A
  receives `ErrDying` from unit B, it means A used B's dying channel, which is
  illegal -- each worker's dying channel is private. Custom sentinels
  (`ErrAPIServerDying`, `ErrChangeStreamDying`, etc.) are used to communicate
  "my upstream dependency has shut down" across worker boundaries cleanly.

### The uniter's specific failure modes

Within the uniter, failure modes are precise:

- **Hook failure** (charm exits non-zero): the operation's Execute phase returns
  `ErrHookFailed`. The executor commits the error state. The unit enters `error`
  status. The resolver returns `ErrWaiting`. The loop idles until `juju
  resolved` clears the error.
- **Reboot requested** (charm calls `juju-reboot`): uniter cleanly stops,
  machine reboots, machine agent restarts, deployer restarts the unit agent,
  uniter resumes from committed state.
- **Unit dying**: controller marks unit Dying, watcher fires, resolver sees the
  lifecycle change and queues the teardown hook sequence (stop, relation-broken,
  storage-detaching, remove), then marks the unit Dead and exits cleanly.

The pattern across all cases is the same: the state machine commits a durable
checkpoint after each phase, so any crash between phases is recoverable by
re-reading the committed state and re-deriving the next operation.

<!-- ILLUSTRATION NEED: Worker lifecycle state machine.
     States: starting → running → dying → dead, with labeled transitions:
     "Kill(nil) = graceful", "Kill(err) = failed", "ErrDying = sentinel exit".
     Possibly a second panel for the uniter's hook execution states:
     pending → preparing → executing → committing → done, with the error path
     branching to "error (hook failed)" and the reboot path branching to
     "rebooting".
     ggarch: the state view is exactly for this. The guard/trigger/action
     attributes on state transitions map precisely to the tomb/catacomb
     semantics. This is an illustration that practically cannot be expressed
     in other docs-as-code tools without layout distortion. -->

---

## Summary: the design principles

The architecture is a coherent consequence of a small set of principles. In
order of precedence:

1. **Declare intent, don't issue commands.** Every operation is a write to the
   controller DB. Agents reconcile toward that written state asynchronously.

2. **State lives in the controller, execution lives in the agents.** The
   controller is the single source of truth. Agents are stateless with respect
   to goal state -- they derive everything from what the controller says.

3. **Notify, don't propagate.** Watchers carry signals, not data. Consumers
   re-read authoritative state after every notification. This is the practical
   implementation of eventual consistency.

4. **Everything connects to the controller; nothing connects peer-to-peer.**
   The star topology of relations is a consequence of this principle, not a
   design choice layered on top of it.

5. **Prefer restart over recovery.** A failed worker is killed and restarted
   cleanly. The at-least-once initial watcher event means a restarted worker
   can always recover correct state without peer communication.

6. **No exceptions.** Every component -- provisioner, machine agent, unit agent,
   charm, controller itself -- follows these rules. The uniformity is what makes
   the system predictable and auditable.
