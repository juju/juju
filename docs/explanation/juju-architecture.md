---
myst:
  html_meta:
    description: "Juju architecture explained: the software Juju is made of, the data model it stores, and the operations that drive a deployment through its life."
---

(juju-architecture)=
# Juju architecture

Juju is a distributed system for deploying and operating applications on cloud infrastructure. At its core, Juju works by keeping a **declared goal state** -- what you want your deployment to look like -- and continuously reconciling the real world towards it.

This document explains how Juju is put together in three parts, each a different facet of the system:

1. {ref}`The software <arch-software>` -- the programs Juju is made of, where they run, and how they communicate.
2. {ref}`The data model <arch-datamodel>` -- what the controller stores: the abstractions that describe a deployment's intended and current state.
3. {ref}`The operations <arch-operations>` -- how the software changes the data model to bring a deployment to life and keep it running.

These three facets are distinct. The software is programs that run; the data model is records in a database; the operations are what connect the two. Keeping them separate makes it always clear whether something is a program, a piece of stored state, or an action.

(arch-software)=
## The software

Juju is made of a set of programs that collaborate. Each runs as one or more processes on real infrastructure -- a machine, a pod, or a user's workstation.

- **The client** -- Any software that implements the Juju client API contract and talks to a controller: the {ref}`juju CLI <juju-cli>`, the Terraform Provider for Juju, Jubilant, and JAAS/JIMM. A client holds no persistent state of its own; it exists to express intent to the controller.
- **The controller process** -- A `jujud` binary running on the controller host or pod. It runs the Juju API server, the controller- and model-level workers, and the in-process database.
- **The agents** -- `jujud` (and `containeragent` on Kubernetes) processes that run on provisioned resources. There are four kinds -- controller, model, machine, and unit agents -- and each drives the reconciliation of one entity.
- **The charm runtime** -- A charm's `dispatch` entry point and the {ref}`hook commands <jujuc>` it calls. The unit agent executes the charm code, and the charm reads and writes its Juju context through hook commands.
- **Pebble** -- A lightweight process supervisor injected into each Kubernetes workload container. It manages the workload's services and files on behalf of the charm.
- **The workload** -- The actual application software the charm operates.

Where these programs run is the **deployment topology**, described next. How they talk to one another is described after that.

(arch-topology)=
### Deployment topology

After you bootstrap a controller and deploy an application -- described in {ref}`the operations <arch-operations>` -- the programs run on infrastructure in a characteristic arrangement. This is what a live deployment looks like.

::::{tab-set}

:::{tab-item} Kubernetes clouds

On a Kubernetes cloud, a live deployment looks like this:

- **Controller pod** -- Runs `jujud`, which hosts the controller and model agents. The Dqlite database runs in-process within `jujud`.
- **Charm pods** (one per unit) -- Each unit pod has a charm container (running the `containeragent` unit agent) and one or more workload containers. Pebble is injected as the init process of each workload container.
- **Storage and network** -- A unit pod draws {ref}`storage <storage>` (a persistent volume) and {ref}`networking <space>` (a space or subnet) from the cluster.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart LR
    subgraph controller_pod["Controller pod"]
        jujud_c["jujud<br/>(controller + model agent workers)"]
    end

    subgraph unit_pod["Unit pod (one per unit)"]
        subgraph charm_container["Charm container"]
            ca["containeragent"]
        end
        subgraph workload_container["Workload container"]
            pebble["Pebble (init)"]
            svc["Workload service(s)"]
        end
    end

    Storage[("Storage (PVC)")]
    Net[("Network space / subnet")]

    jujud_c -. "Juju API (websocket)" .-> ca
    ca -. "Pebble API (HTTP)" .-> pebble
    unit_pod --- Storage
    unit_pod --- Net
```
*A Kubernetes deployment. A controller pod, running `jujud` with the controller and model agent workers, talks to a unit pod over the Juju API. Inside the unit pod, the charm container's `containeragent` manages the workload container via the Pebble API. The unit pod also draws storage, in the form of a persistent volume claim, and networking, in the form of a space or subnet, from the cluster.*

:::

:::{tab-item} Machine clouds

On a machine cloud, a live deployment looks like this:

- **Controller machine** -- Hosts one `jujud` process for the controller and model agent workers, alongside the Dqlite database.
- **Workload machines** -- Each provisioned machine hosts one `jujud` process. It runs the machine agent workers and, nested within them, the unit agent workers for every unit on the machine. Units from different applications can share a machine.
- **System containers** (LXD) -- Juju treats LXD containers as regular machines. A container on machine `0` appears as `0/lxd/0` and has its own `jujud` process with its own machine and unit agent workers.
- **Storage and network** -- A workload machine draws {ref}`storage <storage>` (attached volumes) and {ref}`networking <space>` (a space or subnet) from the cloud.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart LR
    subgraph controller_machine["Controller machine"]
        subgraph jujud_ctrl["jujud process"]
            CA["Controller agent workers"]
            MA["Model agent workers"]
        end
        DB[("Dqlite")]
    end

    subgraph model_machine["Workload machine"]
        subgraph jujud_model["jujud process"]
            MachA["Machine agent workers"]
            subgraph unit_workers["(per unit)"]
                UA["Unit agent workers"]
            end
        end
        Charm[("Charm code")]
    end

    Storage[("Storage volume")]
    Net[("Network space / subnet")]

    CA -.->|"Juju API (websocket)"| UA
    UA -.->|"hook commands (unix socket)"| Charm
    model_machine --- Storage
    model_machine --- Net
```
*A machine cloud deployment. A controller machine runs `jujud` with the controller and model agent workers and the Dqlite database, and talks to a workload machine over the Juju API. On the workload machine, a single `jujud` process hosts the machine agent workers and, nested within them, a set of unit agent workers per unit, which drive the charm code over a Unix socket. The workload machine also draws storage, in the form of an attached volume, and networking, in the form of a space or subnet, from the cloud.*

```{ibnote}
See more: {ref}`machines-and-system-containers`, {ref}`machine`
```

:::

::::

(arch-communication)=
### Communication paths

The programs of a Juju deployment talk to each other over four paths:

- **Client to controller** -- The client connects to the controller over a websocket-based RPC API (the Juju API). This is how a client expresses intent -- for example, to deploy an application or change its configuration.
- **Controller to agents** -- Agents use an event-driven contract built on **watchers**: long-lived API calls that block until a change relevant to that agent occurs, then return a summary of what changed. When the declared state changes, the affected agents are notified and react.
- **Unit agent to charm** -- in two directions: downward (the agent sets environment variables and runs the charm's `dispatch` script as a subprocess), and upward (during a hook the charm calls {ref}`hook commands <hook-command>` -- the `jujuc` binaries), over a Unix socket the unit agent listens on.
- **Charm to workload** -- On Kubernetes, through the **Pebble API**, an HTTP API served by Pebble inside each workload container. On machine clouds, the charm drives its workload directly using standard operating-system mechanisms, since the charm and workload are co-located.

```{ibnote}
See more: {ref}`jujuc`, {ref}`pebble`, {ref}`database`
```

(arch-datamodel)=
## The data model

This section covers what the controller stores. The entities a deployment tracks are **database records**, not running processes: they describe a deployment's intended and current state. They are stored in the model database for each model -- one of the Dqlite stores the controller keeps.

A model in Juju is either a **class model** (created by bootstrap) or a **regular model** (created by a user, holding their workload applications).

The data model:

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
erDiagram
    MODEL ||--|{ APPLICATION : contains
    APPLICATION ||--|{ UNIT : consists of
    APPLICATION ||--|| CHARM : deployed from
    UNIT ||--|| MACHINE : runs on
```
*The core entity relationships in the data model: a model contains applications, each of which consists of units and is deployed from a charm, and each unit runs on a machine.*

### Model

A model is the largest logical container in a deployment. It groups the applications and their supporting components -- machines, storage, networks, relations, and so on -- that work together to deliver a product or service. Every entity Juju manages belongs to exactly one model. A model lives on a controller and is associated with a cloud.

```{ibnote}
See more: {ref}`model`
```

### Application

An application is a running instance of a ycharm inside a model. It lives in a model and consists of one or more units.

```{ibnote}
See more: {ref}`application`
```

### Unit

A unit is a single running instance of the software an application describes. It runs on a machine. An application can have several units, spread across several machines. A unit is named on the pattern `<application>/<unit ID>` -- for example, `mysql/0`.

```{ibnote}
See more: {ref}`unit`
```

### Machine

A machine is what a unit runs on. On machine clouds it is a VM or bare-metal host (or an LXD container); on Kubernetes it is the pod hosting the unit. From Juju's point of view these are all machines, and each is a record in the controller's database.

```{ibnote}
See more: {ref}`machine`
```

### Relation

A relation connects two applications so they can exchange data. The controller records relations and their data bags as part of the data model.

```{ibnote}
See more: {ref}`relation`
```

### Configuration

Each application has a configuration -- a set of key-value settings provided by a user and kept by the controller. The charm reads and uses the configuration.

```{ibnote}
See more: {ref}`configuration`
```

### Secrets

Secrets are versioned sensitive values held by the controller, granted to applications. Charms can create, grant, update, and revoke secrets; users can create and grant user secrets.

```{ibnote}
See more: {ref}`secret`
```

### Status

Status is the current state and message of an application or unit, set by a charm via `status-set`. The controller stores it and serves it on demand.

```{ibnote}
See more: {ref}`status`
```

### Storage

A unit may draw storage from its cloud -- data volumes that survive the machine or pod. Storage is represented in the model.

```{ibnote}
See more: {ref}`storage`
```

### Space and network

A deployment may split traffic across network spaces and subnets. Spaces segment traffic and constrain where units sit.

```{ibnote}
See more: {ref}`space`
```

(arch-operations)=
## Operations

The software and data model sections show what a deployment is. This section shows how the system comes to life and stays in sync: the ideas that make Juju converge (the reconciliation contract), the operations that carry a deployment through its life (the deployment lifecycle), and, at close range, how a single unit runs its charm.

(arch-reconciliation-contract)=
### How Juju converges

The idea behind everything Juju does is **reconciliation**. A client declares what a deployment should look like. The controller persists that declared state in its database. And Juju's agents work to bring the real world into line with what is declared.

Juju's agents are **event-driven**: each subscribes to **watchers** -- long-lived API calls that block until a change relevant to that agent occurs, then return a summary of what changed. When the declared state changes, the affected agents are notified and converge on the new declared state.

Watchers are also what make Juju self-repairing: if something drifts from what is declared -- a machine dies, a unit is removed -- a watcher fires and brings the system back into line.

### The deployment lifecycle

A deployment goes through a small set of recurring operations: it is created (bootstrap), populated (deploy), connected (integrate), scaled and updated, and eventually torn down (remove). A note throughout: the described lifecycle holds for any Juju client. For clarity the client is shown as the `juju` CLI, but the platform behaves the same regardless of client.

#### Bootstrap

Bootstrap is where a client creates a controller on a cloud. It is the first step in any deployment and happens once per deployment. After bootstrap there is one controller, one model, and no workload models.

::::{tab-set}

:::{tab-item} Kubernetes

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

:::

:::{tab-item} Machine

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

::::

```{ibnote}
See more: {ref}`bootstrap-a-controller`
```

#### Deploy

Deploying an application adds its software to the model and arranges for it to run on a resource. The controller writes an application record (with its units) to the database, a model agent asks the cloud for resources (a VM or a chip pod), and once the resource is ready the controller starts the unit agent, which runs the install sequence.

```{ibnote}
See more: {ref}`command-juju-deploy`
```

#### Integrate

Integrating connects one application to another so they can exchange data. When you integrate two applications, the controller writes a relation, the connected unit agents are notified, and each runs its relation hooks -- during which the applications exchange data.

```{ibnote}
See more: {ref}`command-juju-integrate`, {ref}`relation`
```

#### Scale

Scaling changes the number of units of an application. Adding units provisions new resources and starts new unit agents. Scaling reuses the deploy machinery.

```{ibnote}
See more: {ref}`command-juju-scale-application`, {ref}`scaling`
```

#### Upgrade

Upgrading replaces software with a newer version. Two things are upgraded independently: the platform (client, controller, machines) and the applications (a new charm revision). Upgrading a charm installs the new one onto its units and runs the upgrade hooks.

```{ibnote}
See more: {ref}`upgrading-things`
```

#### Remove

Removing tears down all or part of a deployment. Removal is graded: a unit can be removed without touching its interface, an application with its units, or an entire model or controller. At each level the infrastructure is withdrawn from the cloud and the controller cleans up its recorded state.

```{ibnote}
See more: {ref}`removing-things`
```

### How a unit runs

This zooms into the unit agent, the charm, and the workload -- how the unit agent operates, and how the two talk to the controller.

#### The control loop

The unit agent (specifically its uniter worker) runs loops continuously:

1. **Wait** for any watcher to signal a change.
2. **Snapshot** the current remote state from the controller.
3. **Resolve** the diff between the snapshot and the agent's record to decide the next hook.
4. **Dispatch** the hook -- run the charm's `dispatch` script.
5. **Commit** any buffered writes back.
6. Return to step 1.

Only one hook runs at a time per unit. Every hook is dispatched with a set of **environment variables** describing the context (`JUJU_UNIT_NAME`, channel, and so on).

##### Hook dispatch

When the unit agent dispatches a hook, it sets environment variables, runs the charm's `dispatch` script, and listens on Unix socket for {ref}`hook commands <hook-command>`. It serves each command on the charm's behalf, against the controller. On exit code 0 it flushes any buffered writes to the controller; on a non-zero code it discards them and marks the unit `error`.

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
*The unit agent calls the charm's `dispatch` script, which issues hook commands; the unit serves these against the controller; and it commits or discards on exit.*

```{note}
The `update-status` hook fires on a timer (default: five minutes), not in response to a change.
```

#### Data synchronisation during a hook

The controller database is the single source of truth Juju state. When a hook runs, the charm has no in-process memory of previous runs: everything it needs comes from the database, via hook commands, on demand.

The timing of that data flow follows from a few invariants Juju maintains while a hook runs. This section covers those invariants first.

##### What a charm can rely on

Four invariants hold while a hook is running:

- **One hook at a time per machine.** A machine-level lock is held for the whole run of a hook, so hooks of different units on the same machine never interleave. Different machines run independently with no ordering guaranteed.
- **Config is a stable snapshot.** Read once at the start of a hook.
- **Writes are all-or-nothing.** The hook's writes to relation data, secrets, and state are buffered and flushed together on a clean exit -- and discarded on failure. `status-set` is immediate.
- **Leadership is a lease, not a lock.** A successful leadership check guarantees leadership for about 30 seconds. It can change mid-hook.

These invariants are independent: the guarantee of one does not build on another. Convention cancels nothing like "while this hook runs, this unit is leader" or cross-machine ordering.

##### What the controller stores

The controller stores, per model: application configuration, relations (data bags), secrets, application and unit status, leadership, and optionally charm state.

##### When state moves

Timing for each kind of data within a hook:

- **Configuration** -- read once on first use, then cached for the hook.
- **Relation data** -- read lazily, buffered, flushed on clean exit.
- **Secrets** -- read lazily, buffered, flushed on clean exit.
- **Status** -- written immediately on each `status-set`.
- **Leadership** -- checked fresh each time, never cached.
- **Charm state** -- buffered, flushed on clean exit.
- **Action results** -- buffered, flushed on clean exit.

##### The three phases of a hook run

1. **Setup** -- the agent prepares the context (env vars, config cache).
2. **Execute** -- `dispatch` runs; hook commands are served live.
3. **Commit** -- on clean exit, the agent flushes buffered writes; otherwise it discards them.
