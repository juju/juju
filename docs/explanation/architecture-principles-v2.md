---
myst:
  html_meta:
    description: "Juju's architecture from first principles: the constraints that
      force its structure, and why resilience, consistency, and self-healing are
      inevitable consequences rather than designed-in properties."
---

(architecture-principles-v2)=
# Juju architecture: principles


## The problem forces the structure

You want to operate applications on cloud infrastructure -- deploy, configure,
integrate, scale, upgrade, remove -- on any cloud, without writing
cloud-specific or application-specific glue code every time.

Two constraints follow immediately, and they are not negotiable.

**Cloud logic and application logic must stay separate.** Provisioning a machine
from AWS and operating a PostgreSQL database are different kinds of knowledge.
If they are entangled, neither is reusable: the database operator would have to
know AWS, the AWS provisioner would have to know PostgreSQL. Juju keeps them
apart by design: cloud knowledge stays with the cloud provider; application
knowledge stays in **charms**, reusable operators published on Charmhub.

**Intent and execution must stay separate.** At scale -- many applications,
many clouds, machines that disappear, networks that partition -- the person or
system declaring *what should be* cannot also be responsible for knowing *how to
make it so* on every host at every moment. The declaration must be durable and
centrally held; the execution must be distributed and local to each workload.

These two constraints have exactly one structure that satisfies both
simultaneously. There must be a central place that holds the declaration of
intent and knows about both clouds and charms without being responsible for
either. And there must be a set of agents, one beside each workload, that know
only their local piece of the world and drive it toward what the centre says it
should be.

That centre is the **controller**. Those agents are **unit agents** (and on
machine clouds, **machine agents** above them). Everything else in the Juju
architecture is a consequence of this structure, not an independent design
choice.

<!-- ILLUSTRATION NEED: "The forced structure."
     Three columns: cloud knowledge (left), controller (centre), application
     knowledge (right). User intent enters the controller from above; agents
     hang below it, each connected to a workload. The two vertical separations
     (cloud/controller, controller/charm) are the key visual. No arrows between
     cloud and charm -- they never touch.
     ggarch: annotation regions for the two separations. Edge types: "api" for
     user→controller, "control" for controller→agent, "data" for
     agent→workload. -->


## What that structure requires

The controller holds the declared state. Agents execute against it. The
question is: how does an agent know when the declared state has changed?

The controller cannot reach out to agents. If it did, every agent going offline
would block the controller, and the controller's availability would depend on
every host it manages -- exactly the coupling the architecture is designed to
avoid. So agents must connect to the controller, not the other way around.
Each agent holds a long-lived connection and registers **watchers** -- subscriptions
that fire when something relevant to that agent changes in the controller's
database.

This raises a subtler question: when a watcher fires, what does it deliver?

The answer is: **a signal, not data.** A watcher fires an empty notification
("something changed") or at most a list of the IDs of records that changed --
never the changed values themselves. The agent must then call the controller
API to fetch the current state.

This is not an implementation shortcut. It is a correctness property. The
controller's database is replicated and strongly consistent -- changes are
committed via Raft consensus before any watcher fires. But between the moment
a change is committed and the moment the watcher notification arrives at an
agent, further changes may have happened. A notification carrying a value
would carry a potentially stale one. By delivering only a signal and requiring
the agent to fetch, the system guarantees the agent always reads the
authoritative current state, not a snapshot from an arbitrary point in the
past.

One further guarantee makes this workable in practice: every watcher fires
once immediately on creation, before any real changes occur. A freshly started
or restarted agent does not need a special initialisation path -- it subscribes,
receives the baseline signal, fetches current state, and enters the same
reconciliation loop it would use for any subsequent change.

<!-- ILLUSTRATION NEED: "Notify, then pull."
     Two-panel sequence. Panel 1: controller DB changes (user writes intent);
     watcher fires a thin signal arrow to agent; agent sends a fat "fetch"
     arrow back to controller; controller responds with current state; agent
     acts.
     Panel 2 (or annotation): the initial event on watcher creation, showing
     the same flow happening at agent startup with no prior change.
     ggarch: "event" edge type for the watcher signal; "api" edge type for the
     fetch. The visual contrast between thin/async and fat/sync is the point.
     This diagram tests whether ggarch can represent two distinct edge
     semantics clearly in a sequence context. -->


## What follows

Notify-then-pull is a single rule. Its consequences are not separate design
decisions -- they are the same rule showing up at different scales.

### Eventual consistency

The controller database is strongly consistent: Raft consensus, ACID
transactions, durable across restarts. But the propagation of state changes
to agents is not. An agent that is partitioned, restarting, or simply slow
will lag behind the declared state. The system does not treat this as an
error. When the agent reconnects, it receives its baseline watcher event,
fetches current state, and resumes converging. The only invariant is eventual
convergence, not simultaneous convergence.

This makes the system AP in the CAP theorem sense -- available and
partition-tolerant, at the cost of temporary divergence between declared state
and the real world. That tradeoff is deliberate: a deployment spread across
an unreliable network can still make progress, because agents reconcile
independently and the controller is the single meeting point they converge
toward.

### Self-healing and the restart preference

Because a restarted agent can always recover its correct state from the
controller -- no peer needs to re-send anything, no local state needs to
survive the restart -- the system treats failure as the normal operating mode
rather than an exceptional one.

Every worker in an agent is supervised. A failure in one worker kills that
worker and everything that depends on it, then the whole subtree is restarted
cleanly. The preference is always for a clean restart over attempting to
continue with a worker in an uncertain state: a worker that has failed may
have left its internal bookkeeping inconsistent, and its dependents may be
operating on assumptions that no longer hold. Restarting costs a brief
convergence delay; continuing costs correctness. The system chooses the
convergence delay every time.

This supervision is implemented through two Go primitives worth naming because
contributors will encounter them throughout the codebase: a **tomb** manages
the lifetime of a single worker goroutine (kill it, wait for it, detect
shutdown from inside); a **catacomb** manages a worker that itself owns child
workers and watchers, propagating failure upward so a dying child takes down
its parent. Leaf workers use tombs; anything with children uses catacombs.

### The star topology of relations

The notify-then-pull rule applies to relations too, and the result is visible
in the deployment topology: applications integrated with each other never
communicate directly. Every relation goes through the controller.

When unit A writes relation data (via a hook command), it writes to the
controller database. The controller fires a watcher notification to unit B.
Unit B fetches the data from the controller. Unit B writes its own response
to the controller. The controller notifies unit A. At no point do A and B
have a direct connection.

This is not a limitation -- it is the notify-then-pull rule applied to
multi-party state. The controller is the single auditable source of truth for
every integration. Cross-model and cross-controller integrations work by the
same mechanism, because each side speaks only to its own controller.

<!-- ILLUSTRATION NEED: "Star topology of relations."
     Three unit agents (A, B, C) each connected to the controller. Relation
     data bags for A-B and B-C shown as records inside the controller, not
     inside the agents. Watcher notification arrows from controller to agents;
     API write arrows from agents to controller. No direct A-B or A-C edges.
     ggarch: record-style node for the controller to show the data bags as
     named fields. Edge types: "event" for notifications, "data" for writes,
     "api" for reads. Tests whether record nodes compose cleanly with topology
     edges in the same diagram. -->


## How it plays out

The abstract structure -- controller holds intent, agents converge toward it
via notify-then-pull -- plays out as a concrete chain of workers, each
supervising the next.

### The controller: declared state and the provisioner

The controller runs a tree of workers for each model. The one that kicks off
execution is the **compute provisioner**: a model worker that watches the
machine records in the model database. When a deploy or scale-up writes a new
machine record, the provisioner fetches the placement parameters, calls the
cloud's StartInstance API, and records the resulting instance ID back to the
database. It watches; it acts; it declares the result. No other worker needs
to know the instance was created -- they will find out when their own watchers
fire.

On Kubernetes, the equivalent is a pod scheduler manifold rather than a
compute provisioner, but the pattern is identical.

The provisioner must run on exactly one controller node in a high-availability
deployment. It achieves this not through explicit locking but through the
dependency graph: its cloud-provider dependency resolves only on the
responsible controller node, so the manifold never starts on the others.

### The machine agent

The provisioned host boots with the Juju agent binary already installed. The
**machine agent** (`jujud`) starts, connects to the controller, and runs its
own dependency engine -- a tree of workers for storage, log forwarding, machine
health, LXD container provisioning, and a **deployer** worker that watches
which units are assigned to this machine.

This applies to machine clouds (AWS, OpenStack, MAAS, LXD). On Kubernetes
there is no machine agent: each unit runs in its own pod with a
`containeragent` process that combines the machine and unit agent roles.

### The unit agent and the resolver loop

When the deployer sees a unit assigned to the machine, it starts a nested
dependency engine for that unit -- a **unit agent** running inside the machine
agent process. Its central worker is the **uniter**, which implements the
convergence loop.

The uniter fans several watchers (unit lifecycle, application config,
relations, storage, secrets, leadership) into a single coalescing snapshot of
remote state. Then it loops:

- Snapshot the current remote state from the controller.
- Resolve: given the unit's local state and the remote snapshot, what is the
  next operation? (A hook to run, an upgrade to perform, an action to execute,
  or nothing.)
- Execute the operation through a three-phase state machine -- prepare, execute,
  commit -- writing a durable checkpoint after each phase.
- Repeat.

The three-phase checkpoint means a crash at any point leaves the unit in a
known state the resolver can recover from on restart. There is no separate
"discard" path: a failed execute commits the error state, the unit enters
`error` status, and the loop idles until a user clears it with `juju resolved`.

### Charm execution and hook commands

When the resolver decides a hook must run, the uniter opens a Unix domain
socket and starts an in-process RPC server -- the **jujuc server** -- bound to
the current hook's execution context. It then forks the charm process, passing
the socket address and a context ID in the environment.

Every hook command the charm calls (`config-get`, `relation-get`,
`status-set`, and the rest) is a symlink to the `jujuc` client binary. When
the charm calls `relation-get`, `jujuc` dials the Unix socket, sends the
request to the uniter's in-process server, and returns the result. **No hook
command ever reaches the controller directly** -- they all go through the
uniter, which mediates access to the controller API and enforces the hook's
execution context.

On clean exit the uniter's commit phase flushes buffered writes (relation
data, status changes) to the controller. On failure, nothing is flushed. The
hook's effects are atomic with respect to the unit's declared state.

<!-- ILLUSTRATION NEED: "The execution chain."
     Left-to-right or top-to-bottom: controller (with provisioner worker) →
     machine / K8s node → machine agent (with deployer) → unit agent (uniter
     resolver loop) → charm process → jujuc server (back to uniter) → controller
     API.
     Key visual: each arrow is a different kind of connection. Controller to
     cloud: StartInstance (external/cloud call). Machine boots: not a Juju
     connection at all. Machine agent to controller: API (websocket). Deployer
     to unit agent: in-process (no network). Uniter to charm: exec (process
     fork). Charm to jujuc server: Unix socket RPC. Jujuc server to controller
     API: API call.
     ggarch: this is the most demanding diagram in the doc. It requires at
     least five distinct edge types in one view, in-process vs cross-process
     containment, and the K8s vs machine split. Two variants (machine and K8s)
     may be cleaner than one combined diagram. Tests lifecycle (ephemeral for
     the charm process, init for the provisioner's one-shot instance call) and
     in-process containment (unit agent inside machine agent). -->


## The rule with no exceptions

The architecture is a coherent consequence of one rule applied uniformly:

> **Declare state to the controller. Watch for changes. Fetch current state.
> Act. Declare the result.**

Every component follows it without exception -- the provisioner, the machine
agent, the unit agent, the charm via hook commands, and the controller's own
internal workers. The uniformity is not aesthetic. It is what makes the system
auditable, self-healing, and correct across restarts, partitions, and failures:
any component can be restarted from scratch, subscribe to its watchers, and
resume from a coherent state without any peer needing to re-send anything.
