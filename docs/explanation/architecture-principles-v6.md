---
myst:
  html_meta:
    description: "Juju's architecture from first principles: the problem it solves,
      and how intent, persistence, and execution combine to solve it."
---

(architecture-principles-v6)=
# Juju architecture: principles

## The problem

You want to operate applications on cloud infrastructure -- deploy, configure,
integrate, scale, upgrade, remove -- on any cloud, without writing
cloud-specific or application-specific glue code every time.

Two constraints follow immediately.

**Cloud logic and application logic must stay separate.** Provisioning a machine
on AWS and operating a PostgreSQL database are different kinds of knowledge. If
they are entangled, neither is reusable: the database operator has to know AWS;
the AWS provisioner has to know PostgreSQL. Juju keeps them apart. Cloud
knowledge stays with the cloud provider. Application knowledge stays in
**charms** -- reusable operators, one per application, published to Charmhub.

**Intent and execution must stay separate.** At scale -- many applications, many
clouds, machines that disappear, networks that partition -- the person declaring
what should be cannot also be responsible for making it so on every host at
every moment. The declaration must be durable and centrally held. The execution
must be distributed, local to each workload.

These two constraints produce the structure: a **controller** that holds the
declaration of intent and knows about both clouds and charms without owning
either, and **agents** -- one beside each workload -- that execute against it.

## Intent

When you run `juju deploy postgresql`, nothing happens to any machine. The
`juju` CLI sends the intent to the controller. The controller writes a record to
its database: there should be a PostgreSQL application with N units. That record
is the goal state. The database is the single source of truth.

Every operation you perform -- deploy, integrate, scale, upgrade, remove -- is a
write to the controller database. The declaration of intent is always complete
before any execution begins.

## Persistence

The database must be durable. A controller restart, a network partition, a
failed node -- none of these can lose the goal state. The controller database is
strongly consistent: replicated across HA controller nodes via Raft,
ACID-transactional, durable across restarts.

The database is also the *only* place state lives. Agents hold no goal state of
their own -- they derive their behaviour entirely from what the controller says.
This is the property that makes the system self-healing: an agent that restarts
does not need to ask peers what it missed. It reads the controller and converges
from there.

This forces how agents receive changes. The controller never pushes state to
agents. Each agent holds **watcher** connections to the controller that fire
when something relevant changes. A watcher fires a signal -- not the changed
data. The agent then reads the current authoritative state from the controller.
The reason: by the time the notification arrives, further changes may have
happened. The agent always reads what is true now.

Every watcher fires once on creation. A restarting agent subscribes, receives
that baseline signal, reads current state, and enters the same loop it runs for
every subsequent change. Restart and normal operation are the same code path.

```{ggarch}
:file: ../principles.ggarch
:slides: Initial event | Notify then pull
:slide-captions: On creation every watcher fires once immediately. A restarting agent subscribes, receives the baseline signal, and enters the same loop. | Normal cycle. The watcher fires a signal -- no data. The agent fetches current state and reconciles.
:alt: Watcher notification and pull sequence between controller DB and agent.
```

## Execution

Intent in the database must reach a running workload. The path from record to
reality passes through a chain of agents, each watching the output of the one
above it.

The **compute provisioner** -- a worker inside the controller -- watches machine
records in the model database. When a deploy writes a new machine record, the
provisioner calls the cloud API to start an instance and writes the instance ID
back. The cloud now has a running host.

That host boots with the Juju agent binary installed. The **machine agent**
starts, connects to the controller, and watches which application units are
assigned to it. When it sees a new unit, it starts a **unit agent** for that
unit.

A **unit** is a single running instance of an application: one PostgreSQL, one
web server. The unit agent's central worker is the **uniter**, which watches the
unit's lifecycle, config, relations, storage, and leadership, then loops:

- Snapshot current remote state from the controller.
- Resolve: given local state and the remote snapshot, what is the next
  operation? A hook to run, an upgrade, an action, or nothing.
- Execute through a three-phase state machine -- prepare, execute, commit --
  writing a durable checkpoint after each phase.
- Repeat.

**Hooks** are the events a charm responds to: `install`, `config-changed`,
`relation-joined`, `upgrade-charm`, and so on. When the resolver decides a hook
must run, the uniter forks the charm process. During the hook, the charm reads
configuration, fetches relation data, and reports status through **hook
commands** -- small binaries that send requests to an in-process server in the
uniter, which mediates access to the controller API. Hook commands never go to
the controller directly.

On clean exit, the uniter flushes buffered writes -- relation data, status
changes -- to the controller. On failure, nothing is flushed. A crash between
phases leaves the unit in a known state; the resolver re-reads the checkpoint on
restart and continues from there.

```{ggarch}
:file: ../principles.ggarch
:slides: Execution chain IAAS | Execution chain K8s
:slide-captions: Machine cloud. The charm process is ephemeral -- it runs for one hook then exits. The jujuc server is its lifetime peer, mediating all hook command calls. | Kubernetes. The containeragent combines machine and unit agent roles in one process inside the unit pod.
:alt: Execution chain from controller provisioner through to charm process and jujuc server.
```

## The rule

The same pattern runs at every level: watch the controller, read current state,
act, declare the result back. The provisioner does it. The machine agent does
it. The unit agent does it. The charm does it through hook commands. No
component holds goal state of its own; every component derives it from the
controller.

Relations are the clearest demonstration. When unit A writes relation data, it
writes to the controller database. The controller notifies unit B. Unit B reads
the data from the controller and writes its response. The controller notifies
unit A. A and B have no direct connection. Every integration goes through the
controller -- which is the single auditable source of truth for what two
applications have agreed.

```{ggarch}
:file: ../principles.ggarch
:view: Star topology
:caption: Every relation goes through the controller. The data bags live there; unit agents read and write through the controller API and receive watcher notifications from it.
:alt: Controller in the centre top. Three unit agents below it. Each writes data up to the controller and receives event notifications down from it. No direct edges between agents.
```

The uniformity is what makes the system correct across restarts, partitions, and
failures. Restart any component. It subscribes its watchers, receives the
baseline event, reads the controller, finds the gap between what is declared and
what it has done, and closes it. No peer needs to know it was gone. The
declaration you made at the start is what the entire system is always returning
to.
