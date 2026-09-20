---
myst:
  html_meta:
    description: "Juju architecture explained from first principles: the problem
      it solves, and how intent, persistence, and execution combine to solve it."
---

(architecture-principles-v8)=
# Juju architecture

## The problem

You want to operate applications on cloud infrastructure. Deploy them, configure
them, connect them to each other, scale them, upgrade them, remove them -- on
any cloud, without writing cloud-specific or application-specific glue code
every time.

<!-- DIAGRAM 1: the simple statement. A person connected by "operates" to a
workload sitting on a cloud resource. Two nodes, one arrow. No Juju. -->

That simple statement hides a lot. When you unpack what "operate" actually
requires, the picture looks like this:

<!-- DIAGRAM 2: the full Juju chain. Person → client → controller (with cloud
provider components inside, connecting out to clouds; and a connection down to
Charmhub) → agent → charm → workload (on a cloud resource). The entire Juju
chain -- client, controller, agent -- in orange. Charm in white with orange
border. Workload and cloud resource in their own colours. This is the shape of
the problem when fully unpacked. -->

This is the shape of the problem when fully unpacked. Every element in that
picture earns its place. Two constraints force the structure.

**Cloud logic and application logic must stay separate.** Provisioning a machine
on AWS and operating a PostgreSQL database are different kinds of knowledge. If
they are entangled, neither is reusable: the database operator has to know AWS;
the AWS provisioner has to know PostgreSQL. In the diagram, this separation is
visible: cloud knowledge lives in **cloud providers** -- components inside the
controller that speak each cloud's API -- while application knowledge lives in
**charms**, reusable operators published to Charmhub, one per application.
Neither side knows the other exists. When you run `juju deploy postgresql`, the
charm handles the PostgreSQL logic; the controller's cloud provider handles the
provisioning.

**Intent and execution must stay separate.** At any realistic scale -- many
applications, many clouds, machines that disappear, networks that partition --
the person declaring what should be running cannot also be responsible, at every
moment, for making it so on every host. You need something that holds the
declaration durably, and something else, distributed across every host, that
executes against it. Otherwise every failure requires human intervention. In the
diagram: the **controller** holds the declaration; **agents** execute against it,
one beside each workload.

The user talks to the controller through a **client** -- the `juju` CLI, the
Terraform provider, or JAAS. The client is where intent originates.

## Intent

When you run `juju deploy postgresql`, nothing happens to any machine. The
client sends the intent to the controller. The controller writes a record to its
database: there should be a PostgreSQL application with N units. That record is
the goal state. The database is the single source of truth.

This is true for every operation:

- `juju deploy` -- writes application and unit records
- `juju config` -- writes configuration
- `juju integrate` -- writes a relation record connecting two applications
- `juju scale` -- writes new or removed unit records
- `juju upgrade-charm` -- writes a new charm reference
- `juju remove` -- writes a Dying marker on the relevant records
- `juju add-machine` -- writes a machine record

Every operation is a write to the controller database. The declaration of intent
is always complete before any execution begins.

## Persistence

The database must be durable. A controller restart, a network partition, a
failed node -- none of these can lose the goal state. The controller database is
strongly consistent: replicated across HA controller nodes via Raft,
ACID-transactional, durable across restarts.

What lives in the database is worth dwelling on. It holds both physical things
and the abstractions that organise them. A **model** is a self-contained
namespace for a deployment -- a set of applications running on a given cloud,
with their own machines, relations, and storage. A controller manages many
models; each has its own database. Within a model, **applications** and
**units** are records too: an application is a named deployment of a charm; a
unit is a single running instance of that application. The machines and pods
those units run on are records. The relations connecting applications are
records. The charm each application references is a record.

The database is also the *only* place goal state lives. Agents hold none of
their own -- they derive their behaviour entirely from what the controller
currently says. This is the property that makes the system self-healing: an
agent that restarts does not need to ask peers what it missed. It reads the
controller and converges from there.

## Execution

Intent in the database must reach a running workload. The path from record to
reality passes through a chain of agents, each watching the output of the one
above it and acting on what it sees. The pattern is always the same, regardless
of which operation triggered it.

<!-- DIAGRAM: the execution chain. Controller DB → compute provisioner (inside
controller) → provisioned machine/pod → machine agent → unit agent → uniter
loop (snapshot → resolve → execute → commit) → charm process → workload. Each
step labelled with what it watches and what it does. K8s variant as a second
panel: containeragent replaces machine agent + unit agent. -->

The **compute provisioner** -- a worker inside the controller -- watches machine
records. When a deploy or `juju add-machine` writes a new machine record, the
provisioner calls the cloud provider, which calls the cloud API to start an
instance, and writes the instance ID back to the database.

That host boots with the Juju agent binary installed. The **machine agent**
starts, connects to the controller, and watches which units are assigned to it.
When it sees a new unit, it starts a **unit agent** for that unit.

The unit agent's central worker is the **uniter**. It watches everything
relevant to the unit -- lifecycle, config, relations, storage, leadership -- and
runs a continuous reconciliation loop:

- Snapshot current remote state from the controller.
- Resolve: given local state and the remote snapshot, what is the next
  operation?
- Execute, writing a durable checkpoint after each phase.
- Repeat.

The operations are driven by the **charm**: the software operator for that
application. The charm expresses its operational knowledge as **hooks** --
`install`, `config-changed`, `relation-joined`, `upgrade-charm`, and so on.
Each hook is a moment when the charm runs, looks at the current state of the
world, and brings the workload into line with it. Hook calls to the controller
API go through an in-process server in the uniter -- not directly to the
controller. On clean exit, buffered writes flush to the controller. On failure,
nothing flushes. The unit lands in a known state and the resolver picks up from
there on restart.

This same loop handles every operation. A config change fires `config-changed`.
A scale-up starts new unit agents, each running `install` and then `start`. An
upgrade writes a new charm reference; the uniter runs `upgrade-charm`. A removal
marks the unit Dying; the uniter runs the teardown sequence and marks it Dead.
The loop is always the same.

## The rule

Every component -- the provisioner, the machine agent, the unit agent, the charm
-- follows the same pattern: watch the controller, read current state, act,
declare the result back. No component holds goal state of its own.

**Relations** are the most striking demonstration of what this makes possible.
Integrating applications -- connecting a web application to a database, a
service to an observability stack, an application to a secret backend -- is, in
most systems, bespoke glue: custom code that understands both sides and breaks
when either changes. In Juju, `juju integrate` writes a relation record. Unit A
writes its data to the controller. The controller notifies unit B. Unit B reads
the data and writes its response. The controller notifies unit A. A and B have
no direct connection and need none. The controller is the single source of truth
for everything the two sides have agreed. Cross-model and cross-controller
integrations work the same way -- each side speaks only to its own controller.

<!-- DIAGRAM: star topology. Controller in the centre. Three unit agents around
it. Each writes data to the controller and receives notifications from it. No
edges between agents. Labels: "writes relation data", "watcher fires", "reads
relation data". -->

The uniformity is what makes the whole system correct across restarts,
partitions, and failures. Restart any component. It subscribes its watchers,
receives the baseline event, reads the controller, finds the gap between what is
declared and what it has done, and closes it. No peer needs to know it was gone.
The declaration you made at the start is what the entire system is always
returning to.
