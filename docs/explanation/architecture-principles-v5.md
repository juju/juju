---
myst:
  html_meta:
    description: "Juju's architecture from first principles: the problem it solves,
      the structure that solves it, and why the convergence guarantee is the
      inevitable result."
---

(architecture-principles-v5)=
# Juju architecture: principles

## 1. The problem

Operating a fleet of applications on cloud infrastructure -- without the right
tools -- generates three compounding costs.

**Reuse is impossible.** The knowledge of how to operate PostgreSQL -- install
it, configure it, scale it, connect it to other applications -- gets rewritten
for every cloud and every team. Cloud provisioning knowledge gets rewritten for
every application. Neither is portable because both are entangled with each
other and with the specific infrastructure they target.

**Drift is manual work.** There is no durable record of what should be running.
A machine dies, a config changes, a network partitions -- and the running system
diverges from what you intended. Closing that gap means re-issuing commands. The
same work, every time.

**Integration is bespoke glue.** Connecting a web application to a database
requires custom code that understands both sides. It breaks when either side
changes. It cannot be reused.

## 2. The cause

These three costs share a single cause: there is no declaration of intent, only
a sequence of commands.

A command executes and is forgotten. When it finishes, the system holds the
result -- not the intent. When that result drifts, there is nothing to return
to. When you need the same result elsewhere, you adapt the commands and run them
again. When you integrate two applications, you write the glue by hand because
there is no standard channel between them.

## 3. The root

At the root is the absence of a single authoritative record of what the system
should look like. Without one, drift cannot be corrected automatically -- there
is nothing to correct toward. Knowledge cannot be isolated in reusable units --
it is embedded in command sequences that assume their environment. Integration
has no standard mechanism -- every connection is invented from scratch.

## 4. The requirement

To fix this, one thing is required: a central process that holds a durable,
authoritative record of what should be. Call it the **controller**.

That record must be the single source of truth. Not a snapshot of what was
done -- a declaration of what should be, right now. Every operation writes to
it. Every component that executes derives its behaviour from it. When a
component fails and recovers, it reads the record and converges toward it. It
does not ask peers what it missed.

Two separations follow from the reuse requirement. Cloud knowledge -- how to
provision a machine on AWS, schedule a pod on Kubernetes -- belongs with the
cloud provider. Application knowledge -- how to install, configure, integrate,
and upgrade a specific piece of software -- belongs in **charms**: reusable
operators, one per application, published to a shared registry called Charmhub.
Neither side needs to know the other exists. The controller mediates.

## 5. The mechanism

When you run `juju deploy postgresql`, nothing happens to any machine. The
`juju` CLI sends the intent to the controller. The controller writes a record to
its database: there should be a PostgreSQL application with N units. That is the
goal state. The database is strongly consistent -- replicated across controller
nodes via Raft, ACID-transactional, durable across restarts and network
partitions. Every operation -- deploy, integrate, scale, upgrade, remove -- is a
write to it.

**Agents** are the processes Juju runs alongside each workload. Their job is to
close the gap between goal state and reality on their host. They connect to the
controller; the controller never reaches out to them. Each agent holds
**watcher** connections that fire when something relevant changes in the
database. A watcher fires a signal, not data. The agent then reads the current
authoritative state from the controller. By the time a notification arrives,
further changes may have happened -- the agent always reads what is true now,
never a snapshot from the notification.

Every watcher fires once on creation. A restarting agent subscribes, receives
the baseline signal, reads current state, and enters the same loop it runs for
every subsequent change. Restart and normal operation are the same code path.
The agent does not need to know what it missed.

```{ggarch}
:file: ../principles.ggarch
:slides: Initial event | Notify then pull
:slide-captions: On creation every watcher fires once immediately. A restarting agent subscribes, receives the baseline signal, and enters the same loop. | Normal cycle. The watcher fires a signal -- no data. The agent fetches current state and reconciles.
:alt: Watcher notification and pull sequence between controller DB and agent.
```

This pattern runs at every level of the stack. The **compute provisioner** -- a
worker inside the controller -- watches machine records. When a deploy writes a
new machine record, it calls the cloud API, gets an instance, writes the
instance ID back. On the provisioned host, the **machine agent** watches which
application units are assigned to it; a new unit triggers a **unit agent**. The
unit agent watches the unit's lifecycle, config, relations, and storage; it runs
the charm's **hooks** -- `install`, `config-changed`, `relation-joined`, and so
on -- each one bringing the workload into line with declared state. Hook
commands the charm calls during execution are mediated by the unit agent, not
sent directly to the controller.

```{ggarch}
:file: ../principles.ggarch
:slides: Execution chain IAAS | Execution chain K8s
:slide-captions: Machine cloud. The charm process is ephemeral -- it runs for one hook then exits. The jujuc server is its lifetime peer, mediating all hook command calls. | Kubernetes. The containeragent combines machine and unit agent roles in one process inside the unit pod.
:alt: Execution chain from controller provisioner through to charm process and jujuc server.
```

Relations follow the same pattern. When unit A writes relation data, it writes
to the controller database. The controller fires a watcher notification to unit
B. Unit B reads the data from the controller and writes its response. The
controller notifies unit A. A and B have no direct connection. Every integration
goes through the controller. The controller is the single auditable source of
truth for what two applications have agreed.

```{ggarch}
:file: ../principles.ggarch
:view: Star topology
:caption: Every relation goes through the controller. The data bags live there; unit agents read and write through the controller API and receive watcher notifications from it.
:alt: Controller in the centre top. Three unit agents below it. Each writes data up to the controller and receives event notifications down from it. No direct edges between agents.
```

## 6. The payoff

The three costs from the opening resolve by default.

**Reuse.** The PostgreSQL charm works on any cloud because it contains only
application knowledge. The AWS provider handles only provisioning. Neither knows
the other exists.

**Drift.** Every agent is continuously converging toward the goal state in the
controller database. A machine dies -- the provisioner sees the record and
provisions a new one. A process crashes -- the agent restarts, reads current
state, and resumes. No human intervention required.

**Integration.** Every relation goes through the controller. Relation data lives
in the database, not in bespoke glue. The controller is the meeting point both
sides converge toward.

Restart any component anywhere in the stack. It subscribes its watchers,
receives the baseline event, reads the controller, finds the gap, and closes it.
No peer needs to know it was gone. The declaration you made at the start is what
the system is always returning to.
