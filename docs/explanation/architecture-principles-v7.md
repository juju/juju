---
myst:
  html_meta:
    description: "Juju architecture explained from first principles: the problem
      it solves, and how intent, persistence, and execution combine to solve it."
---

(architecture-principles-v7)=
# Juju architecture
## The problem

You want to operate applications on cloud infrastructure -- deploy, configure,
integrate, scale, upgrade, remove -- on any cloud, without writing
cloud-specific or application-specific glue code every time.

One constraint follows immediately: cloud logic and application logic must stay
separate. Provisioning a machine on AWS and operating a PostgreSQL database are
different kinds of knowledge. If they are entangled, neither is reusable: the
database operator has to know AWS; the AWS provisioner has to know PostgreSQL.
Juju keeps them apart. Cloud knowledge stays with the cloud provider.
Application knowledge stays in **charms** -- reusable operators, one per
application, published to Charmhub.

A second constraint follows from scale. At any realistic deployment size --
many applications, many clouds, machines that disappear, networks that partition
-- the person who declared what should be running cannot also be responsible, at
every moment, for making it so on every host. You need something that holds the
declaration durably, and something else, distributed, that executes against it
locally. Otherwise every failure requires you to intervene. The declaration and
the execution must be separate.

These two constraints produce the structure: a **controller** that holds the
declaration of intent and knows about both clouds and charms without owning
either, and **agents** -- one beside each workload -- that execute against it.
The user talks to the controller through a **client** -- the `juju` CLI, the
Terraform provider, or JAAS. The client is where intent originates.

<!-- DIAGRAM: the full cast. User → client → controller (with database, cloud
connections, Charmhub connection) → agents → workloads. Horizontal intent-to-
execution axis. This is the "Juju overview" or "Juju enters" diagram. -->

## Intent

When you run `juju deploy postgresql`, nothing happens to any machine. The
`juju` CLI sends the intent to the controller. The controller writes a record to
its database: there should be a PostgreSQL application with N units. That record
is the goal state. The database is the single source of truth.

Every operation a user performs -- `juju deploy`, `juju integrate`,
`juju scale`, `juju upgrade`, `juju remove`, `juju add-machine` -- is a write
to the controller database. The declaration of intent is always complete before
any execution begins.

## Persistence

The database must be durable. A controller restart, a network partition, a
failed node -- none of these can lose the goal state. The controller database is
strongly consistent: replicated across HA controller nodes via Raft,
ACID-transactional, durable across restarts.

The database is also the *only* place goal state lives. Agents hold none of
their own -- they derive their behaviour entirely from what the controller
currently says. This is the property that makes the system self-healing: an
agent that restarts does not need to ask peers what it missed. It reads the
controller and converges from there.

## Execution

Intent in the database must reach a running workload. The path from record to
reality passes through a chain of agents, each watching the output of the one
above it and acting on what it sees.

<!-- DIAGRAM: the execution chain. Controller DB → compute provisioner →
provisioned host → machine agent → unit agent → uniter loop → charm process.
Each arrow labelled with what the component watches and what it does. -->

The **compute provisioner** -- a worker inside the controller -- watches machine
records in the model database. When a `juju deploy` or `juju add-machine` writes
a new machine record, the provisioner calls the cloud API to start an instance
and writes the instance ID back. The cloud now has a running host.

That host boots with the Juju agent binary installed. The **machine agent**
starts, connects to the controller, and watches which application units are
assigned to it. When it sees a new unit, it starts a **unit agent** for that
unit.

A **unit** is a single running instance of an application: one PostgreSQL, one
web server, one cache node. The unit agent's central worker is the **uniter**.
It watches the unit's declared state -- lifecycle, config, relations, storage,
leadership -- and runs a continuous reconciliation loop:

- Snapshot current remote state from the controller.
- Resolve: given local state and the remote snapshot, what is the next
  operation? If nothing, idle. If something -- run it.
- Execute, writing a durable checkpoint after each phase.
- Repeat.

The operations the uniter runs are driven by the charm. A **charm** is the
software operator for one application: it knows how to install PostgreSQL, apply
a config change, respond when a new relation is formed, handle an upgrade. The
charm expresses this knowledge as **hooks** -- scripts or programs that the
uniter calls at specific moments: `install`, `config-changed`, `relation-joined`,
`upgrade-charm`, and so on. Each hook runs, does its work, and exits. The uniter
calls the next one when the state warrants it.

During a hook, the charm interacts with the controller through **hook commands**
-- `config-get`, `relation-set`, `status-set`, and the rest. These go through
an in-process server in the uniter, not directly to the controller. On clean
exit, the uniter flushes buffered writes to the controller. On failure, nothing
is flushed. A crash mid-hook leaves the unit in a known state; the resolver
re-reads the checkpoint on restart and continues from there.

<!-- DIAGRAM: one unit's reconciliation loop, zoomed in. Controller API →
watcher fires → uniter snapshots state → resolver → hook runs (charm ↔ jujuc
server) → commit flushes to controller. Show the K8s variant (containeragent)
as a note or second panel. -->

## The rule

Every component in the system -- the provisioner, the machine agent, the unit
agent, the charm -- follows the same pattern: watch the controller, read current
state, act, declare the result back. No component holds goal state of its own.
Every component derives it from the controller.

This pattern supports everything Juju does. Provisioning infrastructure happens
this way. Deploying an application happens this way. Scaling -- `juju scale-application` -- adds unit records to the database; the provisioner and unit agents
pick them up. Upgrading a charm writes a new charm reference to the database;
the unit agent's watcher fires and runs the upgrade hooks. Removing a unit
writes a Dying marker; the unit agent runs the teardown hooks and marks it Dead.

**Relations** -- the mechanism by which applications integrate with each other
-- are the most striking example. Connecting a web application to a database, a
service to an observability stack, an application to a secret backend: in Juju
these are all handled by the same machinery, with no bespoke glue code on either
side. When `juju integrate` connects two applications, it writes a relation
record to the controller database. Unit A writes its data to the controller. The
controller notifies unit B. Unit B reads the data and writes its response. The
controller notifies unit A. A and B have no direct connection and never need
one. The controller is the single auditable source of truth for everything the
two sides have agreed. Cross-model and cross-controller integrations work by the
same mechanism -- each side speaks only to its own controller.

<!-- DIAGRAM: star topology. Controller in the centre. Three unit agents around
it, each writing data to and receiving notifications from the controller.
No direct edges between agents. Labels: "writes relation data", "watcher fires",
"reads relation data". -->

The uniformity is what makes the whole system correct across restarts,
partitions, and failures. Restart any component. It subscribes its watchers,
receives the baseline event, reads the controller, finds the gap between what is
declared and what it has done, and closes it. No peer needs to know it was gone.
The declaration you made at the start is what the entire system is always
returning to.
