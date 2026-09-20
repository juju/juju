---
myst:
  html_meta:
    description: "Juju's architecture from first principles: the constraints that
      force its structure, and why resilience, consistency, and self-healing are
      inevitable consequences rather than designed-in properties."
---

(architecture-principles-v3)=
# Juju architecture: principles


## The problem forces the structure

You want to operate applications on cloud infrastructure -- deploy, configure,
integrate, scale, upgrade, remove -- on any cloud, without writing
cloud-specific or application-specific glue code every time.

Two constraints follow immediately.

**Cloud logic and application logic must stay separate.** Provisioning a machine
from AWS and operating a PostgreSQL database are different kinds of knowledge.
If they are entangled, neither is reusable: the database operator would have to
know AWS, the AWS provisioner would have to know PostgreSQL. Juju keeps them
apart: cloud knowledge stays with the cloud provider; application knowledge
stays in **charms**, reusable operators published on Charmhub.

**Intent and execution must stay separate.** At scale -- many applications,
many clouds, machines that disappear, networks that partition -- the person or
system declaring *what should be* cannot also be responsible for knowing *how to
make it so* on every host at every moment. The declaration must be durable and
centrally held; the execution must be distributed and local to each workload.

These two constraints point to a single structure: a central place that holds
the declaration of intent and knows about both clouds and charms without owning
either, and a set of agents, one beside each workload, that know only their
local piece of the world and drive it toward what the centre says it should be.

That centre is the **controller**. Those agents are **unit agents** (and on
machine clouds, **machine agents** above them). Everything that follows is a
consequence of this structure.

```{ggarch}
:file: ../principles.ggarch
:view: Forced structure
:caption: Two separations define the structure. Intent and persistence live in the controller; execution lives in the agents. Cloud knowledge (provisioning) and application knowledge (charms) are kept separate from each other and from the controller.
:alt: User and client on the left. Controller in the centre, connected up to a cloud and down to Charmhub. Agent and workload on the right. Dashed boxes mark intent and persistence (client and controller) and execution (agent and workload).
```

---

## What that structure requires

The controller holds the declared state. Agents execute against it. How does
an agent know when the declared state has changed?

The controller cannot reach out to agents. If it did, every agent going offline
would block the controller, and the controller's availability would depend on
every host it manages -- exactly the coupling the architecture is designed to
avoid. So agents connect to the controller, not the other way around. Each
agent holds a long-lived connection and registers **watchers** -- subscriptions
that fire when something relevant to that agent changes in the controller's
database.

When a watcher fires, what does it deliver?

**A signal.** A watcher fires an empty notification ("something changed") or at
most a list of the IDs of records that changed -- never the changed values
themselves. The agent then calls the controller API to fetch the current state.

The reason: the controller's database commits changes via Raft consensus before
any watcher fires, but further changes may arrive between a commit and the
moment the notification reaches an agent. A notification carrying a value would
carry a stale one. The agent always fetches the current authoritative state.

Every watcher also fires once immediately on creation. A restarting agent
subscribes, receives that initial signal, fetches current state, and enters the
reconciliation loop -- the same loop it runs for every subsequent change.

```{ggarch}
:file: ../principles.ggarch
:slides: Initial event | Notify then pull
:slide-captions: On creation every watcher fires once immediately. A restarting agent subscribes, receives the baseline signal, and enters the same loop. | Normal cycle. The watcher fires a signal -- no data. The agent fetches current state and reconciles.
:alt: Watcher notification and pull sequence between controller DB and agent.
```

---

## What follows

### Eventual consistency

The controller database is strongly consistent: Raft consensus, ACID
transactions, durable across restarts. The propagation of state changes to
agents is eventually consistent. An agent that is partitioned, restarting, or
simply slow will lag behind the declared state. When it reconnects, it receives
its baseline watcher event, fetches current state, and resumes converging. The
invariant is eventual convergence.

This makes the system AP in the CAP theorem sense -- available and
partition-tolerant, at the cost of temporary divergence between declared state
and the real world. A deployment spread across an unreliable network can still
make progress, because agents reconcile independently and the controller is the
single meeting point they converge toward.

### Self-healing and the restart preference

A restarted agent recovers by subscribing its watchers and fetching current
state from the controller. The system is built for this: failure is the normal
operating mode.

Every worker in an agent is supervised. A failure in one worker kills that
worker and everything that depends on it, then the whole subtree restarts
cleanly. A worker that has failed may have left its internal state inconsistent,
and its dependents may be acting on stale assumptions. A clean restart, however
brief the convergence delay, is more reliable.

This supervision is implemented through two Go primitives worth naming because
contributors will encounter them throughout the codebase: a **tomb** manages
the lifetime of a single worker goroutine (kill it, wait for it, detect
shutdown from inside); a **catacomb** manages a worker that itself owns child
workers and watchers, propagating failure upward so a dying child takes down
its parent. Leaf workers use tombs; anything with children uses catacombs.

### The star topology of relations

The notify-then-pull rule applies to relations too. Applications integrated
with each other never communicate directly -- every relation goes through the
controller.

When unit A writes relation data (via a hook command), it writes to the
controller database. The controller fires a watcher notification to unit B.
Unit B fetches the data from the controller. Unit B writes its own response
to the controller. The controller notifies unit A. A and B have no direct
connection.

The controller is the single auditable source of truth for every integration.
Cross-model and cross-controller integrations work by the same mechanism,
because each side speaks only to its own controller.

```{ggarch}
:file: ../principles.ggarch
:view: Star topology
:caption: Every relation goes through the controller. The data bags live there; unit agents read and write through the controller API and receive watcher notifications from it.
:alt: Controller in the centre top. Three unit agents below it. Each writes data up to the controller and receives event notifications down from it. No direct edges between agents.
```

---

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
database. It watches; it acts; it declares the result. Other workers find out
when their own watchers fire.

On Kubernetes, the equivalent is a pod scheduler manifold rather than a
compute provisioner, but the pattern is identical.

The provisioner must run on exactly one controller node in a high-availability
deployment. It achieves this through the dependency graph: its cloud-provider
dependency resolves only on the responsible controller node, so the manifold
never starts on the others.

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
known state the resolver recovers from on restart. A failed execute commits the
error state, the unit enters `error` status, and the loop idles until a user
clears it with `juju resolved`.

### Charm execution and hook commands

When the resolver decides a hook must run, the uniter opens a Unix domain
socket and starts an in-process RPC server -- the **jujuc server** -- bound to
the current hook's execution context. It then forks the charm process, passing
the socket address and a context ID in the environment.

Every hook command the charm calls (`config-get`, `relation-get`,
`status-set`, and the rest) is a symlink to the `jujuc` client binary. When
the charm calls `relation-get`, `jujuc` dials the Unix socket, sends the
request to the uniter's in-process server, and returns the result. Hook
commands go through the uniter, which mediates access to the controller API
and enforces the hook's execution context.

On clean exit the uniter's commit phase flushes buffered writes (relation
data, status changes) to the controller. On failure, nothing is flushed. The
hook's effects are atomic with respect to the unit's declared state.

```{ggarch}
:file: ../principles.ggarch
:slides: Execution chain IAAS | Execution chain K8s
:slide-captions: Machine cloud. The charm process is ephemeral -- it runs for one hook then exits. The jujuc server is its lifetime peer, mediating all hook command calls. | Kubernetes. The containeragent combines machine and unit agent roles in one process inside the unit pod.
:alt: Execution chain from controller provisioner through to charm process and jujuc server.
```

---

## The rule with no exceptions

The architecture is a coherent consequence of one rule applied uniformly:

> **Declare state to the controller. Watch for changes. Fetch current state.
> Act. Declare the result.**

Every component follows it -- the provisioner, the machine agent, the unit
agent, the charm via hook commands, and the controller's own internal workers.
The uniformity is what makes the system auditable, self-healing, and correct
across restarts, partitions, and failures: any component can be restarted from
scratch, subscribe to its watchers, and resume from a coherent state.
