---
myst:
  html_meta:
    description: "Juju's architecture from first principles: the three demands that
      force its structure, and why the convergence guarantee is the inevitable
      consequence."
---

(architecture-principles-v4)=
# Juju architecture: principles

## The three demands

You want to operate applications on cloud infrastructure -- deploy, configure,
integrate, scale, upgrade, remove. Not once, by hand. Across multiple clouds,
in a repeatable way, for many teams, over a long time. Three things follow.

**Declarative and high-level.** You say what should be, not how to make it so.
"Deploy PostgreSQL with three units" should be a statement, not a script. You
should be able to say it once and have it remain true -- through failures,
restarts, and drift.

**Reusable.** The knowledge of how to operate PostgreSQL should be written once
and work on any cloud. The knowledge of how to provision on AWS should not have
to be rewritten for every application. These two kinds of knowledge must stay
separate. Their entanglement is exactly what makes both fragile and
non-reusable.

**Resilient across failures.** Machines die. Networks partition. Processes
restart. The system must converge back to what you declared, on its own, without
requiring you to re-issue commands.

---

## What they force

### Intent: the controller database is goal state

When you run `juju deploy postgresql`, nothing happens to any machine. The
`juju` CLI sends the intent to the **controller** -- a long-running process that
owns a database of goal state. The controller writes a record: "there should be
a PostgreSQL application with N units". That record is the goal state. The
database is the single source of truth. It is strongly consistent -- replicated
across HA controller nodes via Raft, ACID-transactional, durable across restarts
and network partitions.

Every operation you perform as a user -- deploy, integrate, scale, upgrade,
remove -- is a write to the controller database. The declaration of intent is
always complete before any execution begins.

### Separation: charms and cloud providers

Provisioning a machine on AWS and operating a PostgreSQL database are different
kinds of knowledge. If they are entangled, neither is reusable: the database
operator has to know AWS; the AWS provisioner has to know PostgreSQL. Juju keeps
them apart.

Cloud knowledge stays with the cloud provider. Application knowledge stays in
**charms** -- reusable operators published to Charmhub. The PostgreSQL charm
knows how to install, configure, scale, and upgrade PostgreSQL. It works on any
cloud. The controller knows about both clouds and charms without owning either.

### Execution: agents close the gap

The controller holds the goal state. Something must execute against it on every
provisioned host. Those executors are **agents** -- processes Juju runs
alongside each workload. Their job is to close the gap between what the
controller says should be and what is actually running.

The connection runs one way: agents connect to the controller, not the other
way around. The controller never pushes state to agents -- it only fires
**watcher** notifications when something changes. When a watcher fires, it
delivers a signal, not the changed data. The agent then calls the controller API
to fetch the current authoritative state. The reason: by the time the
notification arrives, further changes may have happened. The agent always reads
what is true now, never a snapshot from the notification.

Every watcher fires once immediately on creation. A restarting agent subscribes,
receives that baseline signal, fetches current state, and enters the same
reconciliation loop it runs for every subsequent change. Restart and normal
operation are the same code path.

```{ggarch}
:file: ../principles.ggarch
:slides: Initial event | Notify then pull
:slide-captions: Sequence diagram: On creation every watcher fires once immediately. A restarting agent subscribes, receives the baseline signal, and enters the same loop. | Sequence diagram: Normal cycle. The watcher fires a signal -- no data. The agent fetches current state and reconciles.
:alt: Watcher notification and pull sequence between controller DB and agent.
```

---

## The convergence guarantee

> **The user declares what should be. The controller holds it. Agents watch for
> changes, fetch current state, act, and declare the result back.**

Every component follows this without exception. Because every component follows
it, the system has a guarantee: restart any component -- provision a new
machine, kill an agent, reconnect after a network partition -- and it converges
to correct state. It does not need its peers to resend anything. It reads the
controller, finds the gap, and closes it.

```{ggarch}
:file: ../principles.ggarch
:view: Forced structure
:caption: Topology: Two separations define the structure. Intent and persistence live in the controller; execution lives in the agents. Cloud knowledge (provisioning) and application knowledge (charms) are kept separate from each other and from the controller.
:alt: User and client on the left. Controller in the centre, connected up to a cloud and down to Charmhub. Agent and workload on the right. Dashed boxes mark intent and persistence (client and controller) and execution (agent and workload).
```

---

## The guarantee at every scale

### Provisioning

The **compute provisioner** is a worker inside the controller. It watches
machine records in the model database. When a deploy writes a new machine
record, the provisioner calls the cloud API to start an instance and writes the
instance ID back to the database. It watches; it acts; it declares the result.
If it restarts, it re-reads the database and resumes. No state is held outside
the controller.

### The execution chain

The provisioned host boots with the Juju agent binary installed. The **machine
agent** (`jujud`) starts, connects to the controller, and runs a tree of
workers. One of those workers -- the **deployer** -- watches which application
units are assigned to this machine. When it sees a new unit, it starts a nested
worker tree: the **unit agent**.

A **unit** is a single running instance of an application: one PostgreSQL
process, one web server. The unit agent's job is to keep that unit converging
toward its declared state.

The unit agent's central worker is the **uniter**. It fans several watchers
(unit lifecycle, config, relations, storage, secrets, leadership) into a single
coalescing snapshot of remote state, then loops:

- Snapshot current remote state from the controller.
- Resolve: given local state and the remote snapshot, what is the next
  operation? A hook to run, an upgrade, an action, or nothing.
- Execute through a three-phase state machine -- prepare, execute, commit --
  writing a durable checkpoint after each phase.
- Repeat.

**Hooks** are the events a charm responds to: `install`, `config-changed`,
`relation-joined`, `upgrade-charm`, and so on. Each hook is a point at which
charm code runs and brings the workload into line with declared state.

A crash between phases leaves the unit in a known state. The resolver re-reads
the checkpoint on restart and continues from there.

```{ggarch}
:file: ../principles.ggarch
:slides: Execution chain IAAS | Execution chain K8s
:slide-captions: Topology: Machine cloud. The charm process is ephemeral -- it runs for one hook then exits. The jujuc server is its lifetime peer, mediating all hook command calls. | Topology: Kubernetes. The containeragent combines machine and unit agent roles in one process inside the unit pod.
:alt: Execution chain from controller provisioner through to charm process and jujuc server.
```

### Hook execution and the jujuc server

When the resolver decides a hook must run, the uniter opens a Unix domain socket
and starts an in-process RPC server -- the **jujuc server** -- bound to the
current hook's execution context. It then forks the charm process to run the
hook.

During the hook, the charm reads configuration, fetches relation data, and
reports status. Every **hook command** (`config-get`, `relation-get`,
`status-set`, and the rest) dials the Unix socket and sends the request to the
jujuc server, which mediates access to the controller API. Hook commands never
go to the controller directly.

On clean exit, the uniter's commit phase flushes buffered writes -- relation
data, status changes -- to the controller. On failure, nothing is flushed. The
hook's effects are atomic with respect to the unit's declared state.

### Relations and the star topology

**Relations** are how applications integrate: a web application connecting to
a database, a service connecting to an observability stack.

The same pattern applies. When unit A writes relation data, it writes to the
controller database. The controller fires a watcher notification to unit B. Unit
B fetches the data from the controller. Unit B writes its response. The
controller notifies unit A. A and B have no direct connection.

Every relation goes through the controller. Relation data lives in the
controller database. The controller is the single auditable source of truth for
every integration state. Cross-model and cross-controller integrations work by
the same mechanism: each side speaks only to its own controller.

```{ggarch}
:file: ../principles.ggarch
:view: Star topology
:caption: Topology: Every relation goes through the controller. The data bags live there; unit agents read and write through the controller API and receive watcher notifications from it.
:alt: Controller in the centre top. Three unit agents below it. Each writes data up to the controller and receives event notifications down from it. No direct edges between agents.
```

### Eventual consistency and self-healing

The controller database is strongly consistent: Raft, ACID transactions, durable
across restarts. The propagation of state changes to agents is eventually
consistent. An agent that is partitioned, restarting, or slow lags behind
declared state. When it reconnects, it receives its baseline watcher event,
fetches current state, and resumes converging.

Every worker in an agent is supervised. A failure in one worker kills that
worker and everything depending on it; the subtree restarts cleanly. This is the
preferred path, not a fallback. A failed worker may have left internal state
inconsistent. A clean restart, reading current state from the controller, is
more reliable than recovery in place.

The Go primitives behind this are worth naming because contributors will
encounter them throughout the codebase. A **tomb** manages the lifetime of a
single worker goroutine. A **catacomb** manages a worker that owns child workers
and watchers, propagating failure upward. Leaf workers use tombs; anything with
children uses catacombs.

---

## The payoff

You declared what should be. That declaration -- a record in the controller
database -- is what every component converges toward. The provisioner reads it
and provisions the host. The machine agent reads it and starts the unit agent.
The uniter reads it and runs the hooks. The charm reads it through hook commands
and configures the workload. Each component declares its result back to the
controller, which fires watchers to the next component in the chain.

Restart any component. It subscribes its watchers, receives the baseline event,
reads the controller, finds the gap, and closes it. No peer needs to know it was
gone.

The declaration reached every component in the system, survived every failure,
and converged -- without you issuing a single command after the first one.
