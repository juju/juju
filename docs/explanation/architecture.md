---
myst:
  html_meta:
    description: "Juju architecture: the problem it solves, the deployment model,
      and the mechanisms that keep the world matching what you declared."
---

(architecture)=
# Juju architecture

## Problem

You want to operate applications on cloud infrastructure -- deploy them, configure
them, connect them to each other, keep them running through upgrades and failures,
and do all of this across multiple clouds. The cast is simple: a **user**, one or
more **infrastructure providers** (AWS, OpenStack, Kubernetes, MAAS, LXD), the
**infrastructure** they supply (machines, pods, storage, networking), and the
**applications** that run on that infrastructure.

Operating this system requires two kinds of expert knowledge that have nothing to do
with each other. The first is cloud knowledge: how to provision a machine on
OpenStack, how to schedule a pod on Kubernetes, how to attach a volume on AWS. The
second is application knowledge: how to install PostgreSQL, how to scale it, how to
upgrade it, how to wire it to another application that may live on a completely
different cloud. These two kinds of knowledge must both be present, but if they are
tangled -- if your database operator has to know OpenStack, or your cloud provisioner
has to know PostgreSQL -- neither is reusable and both are fragile.

In production, applications never run in isolation either. A workload needs to
integrate with observability, identity, secret management, databases, and more -- and
those may each live on different infrastructure. Without a shared integration
mechanism, every connection is bespoke glue code written and maintained by hand.

## Insight

What if you could say "run PostgreSQL on AWS, run this web application on
Kubernetes, connect them" -- and have something else work out what that means for
each cloud, and for each application? That is what Juju does.

The key insight is the separation of two concerns that are usually tangled. Cloud
knowledge -- how to get a machine, attach storage, configure networking -- stays
on the cloud side. Application knowledge -- how to install, configure, integrate,
scale, and upgrade a specific piece of software -- stays on the application side.
Neither bleeds into the other.

The second separation is between **intent** and **execution**. You declare what
you want; Juju works out what needs to happen and drives it to completion. When you
run `juju deploy postgresql`, Juju provisions the infrastructure, places the
application on it, runs the installation sequence, and keeps the result healthy --
all from a single declaration. The same declaration works on AWS, OpenStack, or a
Kubernetes cluster, because the cloud-specific execution is handled below the line
you draw.

## Mechanism

### Topology

A live Juju deployment has a **controller** -- the management process that holds
all goal state -- and one or more **applications**, each broken into one or more
**units**. A unit is the atomic instance of an application: one running copy, on
one machine or pod.

:::::{tab-set}

::::{tab-item} Kubernetes

On Kubernetes, the controller runs in a pod of its own. Each unit runs in a
unit pod with two containers: a charm container (running the `containeragent`
unit agent) and a workload container (running the application itself, with
Pebble injected as the init process to manage workload services).

```{d2}
direction: right

user.shape: person
user.label: "User"

client: "Client" {
  style.fill: "#E95420"
  style.font-color: white
}

controller_pod: "Controller pod" {
  charm_container: "Charm container" {
    charm: "charm" {
      style.fill: white
      style.stroke: "#E95420"
    }
  }
  apiserver: "API-server container" {
    pebble_ctrl: "Pebble (init)" {
      style.fill: "#E95420"
      style.font-color: white
    }
    jujud: "jujud\n(controller + model agent workers)" {
      style.fill: "#E95420"
      style.font-color: white
    }
  }
}

unit_pod: "Unit pod (one per unit)" {
  charm_container: "Charm container" {
    containeragent: "containeragent" {
      style.fill: "#E95420"
      style.font-color: white
    }
  }
  workload_container: "Workload container" {
    pebble: "Pebble (init)" {
      style.fill: "#E95420"
      style.font-color: white
    }
    workload: "Workload service(s)" {
      style.fill: "#4A90D9"
      style.font-color: white
    }
  }
}

storage: "Storage (PVC)" {shape: cylinder}
network: "Network space / subnet" {shape: cylinder}

user -> client: "intent"
client -> controller_pod.apiserver.jujud: "Juju API"
controller_pod.apiserver.jujud -> unit_pod.charm_container.containeragent: "Juju API (websocket)"
unit_pod.charm_container.containeragent -> unit_pod.workload_container.pebble: "Pebble API (HTTP)"
unit_pod -> storage
unit_pod -> network
```
*A Kubernetes deployment. The user expresses intent via a client; the client calls
the Juju API on the controller pod. The controller's `jujud` drives the unit pod's
`containeragent` over the Juju API; the `containeragent` manages workload services
via the Pebble API.*

::::

::::{tab-item} Machine clouds

On a machine cloud, the controller runs on a dedicated machine. Each unit runs on
its own provisioned machine (VM or bare metal), hosting a single `jujud` process
that runs both the machine agent and the unit agent.

```{d2}
direction: right

user.shape: person
user.label: "User"

client: "Client" {
  style.fill: "#E95420"
  style.font-color: white
}

controller_machine: "Controller machine" {
  jujud_ctrl: "jujud process" {
    style.fill: "#E95420"
    style.font-color: white
    ca: "Controller agent workers"
    ma: "Model agent workers"
    db: "Dqlite" {shape: cylinder}
  }
}

workload_machine: "Workload machine" {
  jujud_unit: "jujud process" {
    style.fill: "#E95420"
    style.font-color: white
    mach_a: "Machine agent workers"
    unit_a: "Unit agent workers"
  }
  charm: "Charm code" {
    style.fill: white
    style.stroke: "#E95420"
  }
}

storage: "Storage volume" {shape: cylinder}
network: "Network space / subnet" {shape: cylinder}

user -> client: "intent"
client -> controller_machine.jujud_ctrl.ca: "Juju API"
controller_machine.jujud_ctrl.ca -> workload_machine.jujud_unit.unit_a: "Juju API (websocket)"
workload_machine.jujud_unit.unit_a -> workload_machine.charm: "exec dispatch"
workload_machine.charm -> workload_machine.jujud_unit.unit_a: "hook commands"
workload_machine -> storage
workload_machine -> network
```
*A machine cloud deployment. The controller machine runs `jujud` with controller
and model agent workers and an in-process Dqlite database. Each workload machine
runs its own `jujud` process with machine and unit agent workers, which dispatch
the charm via exec; the charm calls back over a Unix socket using hook commands.*

```{ibnote}
See more: {ref}`machines-and-system-containers`, {ref}`machine`
```

::::

:::::

### Control flow

Every change in a Juju deployment follows the same path through three stages. The
components are introduced here in the order they play their role.

A **client** -- the `juju` CLI, the Terraform provider, or JAAS -- is where intent
enters the system. The client speaks to the controller over a websocket-based RPC
API. It holds no state of its own; it exists solely to express what the user wants.

The **controller** receives that intent and writes it to its database as new goal
state. The database is the single source of truth for the entire deployment. Goal
state is durable -- controller restarts, network partitions, and machine reboots do
not lose it. The controller also talks to **Charmhub** to fetch **charms** -- the
software operators that encode application knowledge -- and to the cloud to provision
infrastructure.

```{ibnote}
See more: {ref}`database`
```

**Agents** close the gap between goal state and the real world. Every provisioned
host runs an agent process (`jujud` on machine clouds, `containeragent` on
Kubernetes). Agents hold long-lived watcher connections to the controller API. When
goal state changes, the relevant watchers fire; the agent diffs the new goal state
against current reality and acts to close the gap. This is also what makes Juju
self-healing: if something drifts from declared state -- a machine dies, a unit
crashes -- the same watcher loop brings it back.

At the unit level, reconciliation works like this: the **unit agent** waits for a
watcher to fire, snapshots remote state from the controller, resolves which hook to
run next, and dispatches it by exec-ing the charm's `dispatch` script. During the
hook, the **charm** calls back to the unit agent over a Unix socket using
{ref}`hook commands <hook-command>` -- this is how it reads configuration, writes
relation data, and reports status. On clean exit the agent flushes buffered writes
to the controller; on failure it discards them and marks the unit `error`. Then the
loop starts again.

```{mermaid}
flowchart TB
    W["Wait\n(watcher)"]
    S["Snapshot\n(remote state)"]
    R["Resolve\n(next hook)"]
    D["Dispatch\n(run charm)"]
    C["Commit\n(flush writes)"]

    W --> S --> R --> D --> C --> W
```
*The unit agent's control loop.*

On Kubernetes, the charm does not drive the workload directly. Instead it talks to
**Pebble** -- a lightweight process supervisor running inside the workload container
-- via an HTTP API. Pebble manages the workload's services and files on the charm's
behalf. On machine clouds the charm drives the workload directly, since charm and
workload share the same machine.

```{mermaid}
sequenceDiagram
    participant Controller
    participant UA as Unit agent
    participant Dispatch as dispatch script
    participant Jujuc as hook commands

    Controller->>UA: watcher fires
    UA->>UA: snapshot
    UA->>Dispatch: exec dispatch
    loop during hook
        Dispatch->>Jujuc: calls hook command
        Jujuc->>Controller: serves via API
        Controller-->>Jujuc: response
        Jujuc-->>Dispatch: return
    end
    alt exit 0
        Dispatch-->>UA: success
        UA->>Controller: flush writes
    else
        Dispatch-->>UA: failure
        UA->>Controller: discard writes, unit error
    end
```
*The unit agent execs the charm's `dispatch` script. During the hook the charm calls
hook commands; the unit agent serves each one against the controller. On clean exit
it flushes buffered writes; on failure it discards them and marks the unit `error`.*

```{note}
The `update-status` hook fires on a timer (default: five minutes), not in response
to a watcher change.
```

```{ibnote}
See more: {ref}`hook-execution-guarantees`
```

### Data model

The controller runs two kinds of database: a **controller database** shared across
all models, and one **model database** per model.

The controller database holds the records that span models: clouds and credentials,
users and access rights, the model registry (one record per model, each pointing to
its own database), SSH keys, and secret backend configuration.

Each model database holds the records for everything running in that model:

- **Application records** -- charm reference, constraints, configuration.
- **Unit records** -- which application, which charm revision, current lifecycle
  state.
- **Machine or pod records** -- hardware constraints, placement, provider ID.
- **Relation records** -- which endpoints are connected; the data bags each side
  has written.
- **Secret records** -- secret metadata and, where the controller is the backend,
  the secret content.
- **Storage, resource, and port records** -- per unit, as applicable.
- **Charm records** -- mostly a pointer: source (Charmhub or local), revision,
  architecture, object-store UUID of the archive, plus separate `charm_*` records
  for the declarations the charm makes (endpoints, actions, config schema, storage
  specs, resource specs).

Every entity in the model has a **lifecycle state**: Alive, Dying, or Dead. Dying
is set by the controller when removal is requested; the relevant agent drives the
entity to Dead by running the teardown sequence. Dead entities are cleaned up by the
controller. This is what makes removal safe and observable: nothing is deleted
until the agent has confirmed it is done.

```{ibnote}
See more: {ref}`controller`, {ref}`model`, {ref}`application`, {ref}`unit`,
{ref}`relation`, {ref}`secret`, {ref}`database`
```

## Instances

The sections below trace how the mechanism plays out for each operation. Each one
is a specific incarnation of the same pattern: the client expresses intent, the
controller writes it to the database, agents reconcile.

### Bootstrap

Bootstrap is where a client creates a controller on a cloud. It is the first step
in any deployment and happens once. The client authenticates with the cloud,
provisions the controller host, installs `jujud`, and waits for the API server and
database to initialise. After bootstrap there is one controller, one model, and no
workload applications.

:::::{tab-set}

::::{tab-item} Kubernetes

```{mermaid}
sequenceDiagram
    actor User
    participant CLI as juju CLI
    participant K8s as Kubernetes cluster
    participant Controller as Controller pod

    User->>CLI: juju bootstrap
    CLI->>K8s: Authenticate
    K8s-->>CLI: OK
    CLI->>K8s: Create namespace + deploy controller pod
    K8s-->>CLI: Pod scheduled
    Controller->>Controller: Start jujud
    Controller->>Controller: Start API server
    Controller->>Controller: Initialise database
    Controller-->>CLI: API ready
    CLI-->>User: Bootstrap complete
```
*Bootstrapping a controller on a Kubernetes cloud. The user invokes `juju bootstrap`. The CLI authenticates with the Kubernetes cluster, deploys the controller pod, waits for the pod to schedule, and receives confirmation once `jujud` has started the API server and initialised the database.*

::::

::::{tab-item} Machine

```{mermaid}
sequenceDiagram
    actor User
    participant CLI as juju CLI
    participant Cloud as Machine cloud
    participant Controller as Controller machine

    User->>CLI: juju bootstrap
    CLI->>Cloud: Authenticate
    Cloud-->>CLI: OK
    CLI->>Cloud: Provision VM
    Cloud-->>CLI: VM ready
    CLI->>Controller: Install jujud + seed config
    Controller->>Controller: Start controller agent
    Controller->>Controller: Start API server + database
    Controller-->>CLI: API ready
    CLI-->>User: Bootstrap complete
```
*Bootstrapping a controller on a machine cloud. The user invokes `juju bootstrap`. The CLI authenticates against the cloud, provisions a virtual machine, installs `jujud`, and the controller then starts its API server and database.*

:::::

```{ibnote}
See more: {ref}`bootstrap-a-controller`
```

#### The controller unit

The controller is itself a deployed unit -- but only in structure, not in how it is
reconciled. On Kubernetes it runs as the `juju-controller` application with a single
unit (`controller-0`) in a pod, with a `charm` container and an `api-server`
workload container (with Pebble inside it supervising the `jujud` controller-agent
service). On a machine cloud it is a unit on the controller machine. Its charm,
`juju-controller`, is a real charm record -- you can see it and upgrade it like any
charm.

But the controller is not operated the way application charms are. For an ordinary
unit, reconciliation means the unit agent dispatches the charm's hooks. For the
controller, the charm is recorded but not dispatched: what runs the controller is
the `jujud` process itself, whose worker tree serves the API, runs the model
workers, and holds the in-process Dqlite store. The `application_controller` marker
flags the controller application as non-ordinary so it is not treated as a workload
charm you can freely integrate and drive via hooks.

The controller is a unit in form, but the loop that keeps it running is `jujud`'s
own machinery, not the charm loop -- the one asymmetry in the unit model.

### Deploy

Deploying an application adds its software to the model and arranges for it to run
on infrastructure. The intent -- application name, charm, constraints -- goes to the
controller as an RPC call. The controller writes application and unit records to the
database, then a model agent asks the cloud for resources (a VM or pod). Once the
resource is ready the controller starts the unit agent, which runs the install
sequence: `install`, `config-changed`, `start`.

:::::{tab-set}

::::{tab-item} Kubernetes

```{mermaid}
sequenceDiagram
    actor User
    participant CLI as juju CLI
    participant Controller as Controller
    participant K8s as Kubernetes cluster
    participant UA as containeragent (unit)

    User->>CLI: juju deploy
    CLI->>Controller: Deploy RPC call
    Controller->>Controller: Write application + unit records
    Controller->>K8s: Schedule unit pod
    K8s-->>Controller: Pod running
    Controller->>UA: Start containeragent
    UA->>Controller: install hook
    UA->>Controller: config-changed hook
    UA->>Controller: start hook
    UA-->>Controller: unit active
    Controller-->>CLI: deploy complete
    CLI-->>User: Application deployed
```
*Deploying on Kubernetes. The CLI sends a deploy call to the controller, which writes the application and unit records and schedules the unit pod. Once the pod is running the controller starts the unit agent, which runs the installation hooks (`install`, `config-changed`, `start`) and reports active.*

::::

::::{tab-item} Machine

```{mermaid}
sequenceDiagram
    actor User
    participant CLI as juju CLI
    participant Controller as Controller
    participant Cloud as Machine cloud
    participant UA as jujud (unit agent)

    User->>CLI: juju deploy
    CLI->>Controller: Deploy RPC call
    Controller->>Controller: Write application + unit records
    Controller->>Cloud: Provision machine
    Cloud-->>Controller: Machine ready
    Controller->>UA: Start jujud (machine + unit agent)
    UA->>Controller: install hook
    UA->>Controller: config-changed hook
    UA->>Controller: start hook
    UA-->>Controller: unit active
    Controller-->>CLI: deploy complete
    CLI-->>User: Application deployed
```
*Deploying on a machine cloud. The CLI sends a deploy call to the controller, which writes the application and unit records and asks the cloud to provision a machine. Once the machine is ready the controller installs and starts `jujud`, which runs the installation hooks (`install`, `config-changed`, `start`) and reports active.*

::::

::::::

```{ibnote}
See more: {ref}`command-juju-deploy`
```

### Integrate

Integrating connects two applications so they can exchange data. The intent -- which
two applications to connect -- goes to the controller, which writes a relation record
to the database. The unit agents on both sides see their watchers fire and run the
relation hooks in sequence. Crucially, the two applications never communicate
directly: all relation data flows through the controller. Each unit writes its data
bag to the controller; the controller notifies the other unit via its watcher. The
controller is always the single source of truth for what two applications have
agreed upon.

```{mermaid}
sequenceDiagram
    actor User
    participant CLI as juju CLI
    participant Controller as Controller
    participant UA1 as Unit agent (app A)
    participant UA2 as Unit agent (app B)

    User->>CLI: juju integrate A B
    CLI->>Controller: Integrate RPC call
    Controller->>Controller: Write relation record
    Controller-->>UA1: watcher fires
    Controller-->>UA2: watcher fires
    UA1->>Controller: relation-created hook
    UA2->>Controller: relation-created hook
    UA1->>Controller: relation-joined hook
    UA2->>Controller: relation-joined hook
    UA1->>Controller: relation-changed hook
    UA2->>Controller: relation-changed hook
    UA1->>Controller: write relation data (app A bag)
    Controller-->>UA2: watcher fires (data changed)
    UA2->>Controller: relation-changed hook
    UA2->>Controller: write relation data (app B bag)
    Controller-->>UA1: watcher fires (data changed)
    UA1->>Controller: relation-changed hook
```
*Integrating two applications. Every arrow passes through the controller -- UA1 and UA2 never communicate directly. The controller holds the relation record; each unit writes its data bag to the controller, which notifies the other via a watcher.*

```{ibnote}
See more: {ref}`command-juju-integrate`, {ref}`relation`
```

### Scale

Scaling changes the number of units of an application. Adding units provisions new
resources and starts new unit agents, reusing the same deploy machinery. Removing
units drives the affected unit agents through the teardown hook sequence.

```{ibnote}
See more: {ref}`command-juju-scale-application`, {ref}`scaling`
```

### Upgrade

Upgrading replaces software with a newer version. Two things upgrade independently:
the platform (client, controller, agents) and the applications (a new charm
revision). Upgrading a charm writes a new charm reference to the database; the unit
agent's watcher fires and it runs the upgrade-charm hook with the new code.

```{ibnote}
See more: {ref}`upgrading-things`
```

### Remove

Removing tears down all or part of a deployment. Removal is graded: a unit can be
removed without touching its application, an application with all its units, or an
entire model or controller. At each level the controller writes a Dying marker to
the database; the relevant agents see their watchers fire and drive the teardown
sequence; infrastructure is released and records are deleted.

Two levels have non-obvious sequences worth understanding for troubleshooting: unit
removal and model removal.

#### Unit removal

Once the controller marks the unit Dying, the unit agent runs the teardown hooks:
`stop` first, then `storage-detaching` and `relation-broken` (each preceded by
`relation-departed` for every known remote unit) in any order, then `remove` last.
The unit is then marked Dead and the controller releases the machine if no longer
needed. A hook failure at any stage leaves the unit in `error` state and blocks
further progress.

```{mermaid}
sequenceDiagram
    actor User
    participant Controller as Controller
    participant UA as Unit agent
    participant Cloud as Cloud

    User->>Controller: juju remove-unit
    Controller->>Controller: Mark unit Dying
    Controller-->>UA: watcher fires
    UA->>Controller: stop hook
    note over UA,Controller: storage-detaching (per storage) and relation-broken<br/>(per relation, preceded by relation-departed per remote unit)<br/>run in any order
    UA->>Controller: remove hook
    UA->>Controller: Mark unit Dead
    Controller->>Cloud: Release machine (if no other units)
    Controller->>Controller: Delete unit records
```
*Unit removal. The controller marks the unit Dying; the unit agent runs the teardown
hooks and marks the unit Dead; the controller releases the machine and cleans up
records.*

```{note}
Use `juju resolved` to retry or skip a failed hook. Use `juju remove-unit --force`
to bypass hooks entirely -- but forced removal may leave orphaned relation data or
unreleased storage on the cloud side.
```

#### Model removal

Model removal is orchestrated by the undertaker worker, which watches for models set
Dying and drives them through to deletion of both the model records and the model's
Dqlite database.

```{mermaid}
sequenceDiagram
    actor User
    participant Controller as Controller
    participant Undertaker as Undertaker worker
    participant Cloud as Cloud

    User->>Controller: juju destroy-model
    Controller->>Controller: Mark model Dying
    Controller-->>Undertaker: watcher fires
    Undertaker->>Controller: Destroy all applications
    note over Controller,Cloud: each unit goes through its removal sequence
    Controller->>Cloud: Release all machines
    Controller->>Controller: Mark model Dead
    Undertaker->>Controller: Delete model records
    Undertaker->>Controller: Delete model Dqlite database
```
*Model removal. The undertaker worker watches for the model to be set Dying, destroys
all applications (each unit going through its own removal sequence), waits for all
machines to be released, then deletes the model records and its Dqlite database.*

```{note}
A `juju destroy-model` that hangs is almost always a unit stuck in its removal hook
sequence. Use `juju status` on the dying model to identify the blocked unit, then
`juju resolved` or `juju remove-unit --force` to unblock it.
```

```{ibnote}
See more: {ref}`removing-things`
```
