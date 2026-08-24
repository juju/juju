---
myst:
  html_meta:
    description: "Juju architecture explained: the state Juju stores, the software that acts on it, and the operations that drive a deployment through its life."
---

(juju-architecture)=
# Juju architecture

Juju is a system for deploying and operating applications on cloud infrastructure. Its central idea is reconciliation: you declare what you want, Juju stores that declaration, and a set of programs continuously drive the real world toward it. Everything in Juju's architecture -- the databases, the processes, the communication patterns -- follows from that single loop.

The key actors are:

- **The client**: Any program that talks to a Juju controller: the `juju` CLI, the Terraform provider, JAAS. Clients hold no state; they express intent.
- **The controller**: The Juju control plane. A process running on a cloud machine or pod that holds the API server, the databases, and the workers that drive reconciliation.
- **The cloud**: The infrastructure provider (AWS, GCP, MAAS, MicroK8s, ...) that the controller talks to in order to provision machines or pods.
- **The model**: A named deployment environment -- a workspace containing a set of applications, the machines they run on, and everything connecting them. A controller manages one or more models.
- **The charm**: The software package that tells Juju how to operate an application: install it, configure it, integrate it with others, upgrade it, remove it.
- **The agent**: A process that runs alongside each machine or unit and drives the reconciliation loop for that entity by watching for state changes and running charm code.

This document goes deeper on each of these, in three parts:

1. {ref}`The data model <arch-datamodel>` -- what Juju stores and how it is organised.
2. {ref}`The software <arch-software>` -- the programs that read and write that state.
3. {ref}`The operations <arch-operations>` -- how state changes over time.

(arch-datamodel)=
## The data model

The controller stores its state in two databases using **Dqlite**, a distributed SQLite implementation embedded in the controller process. The **controller database** holds entities shared across all models: clouds, credentials, users, and the model registry itself. Each model has its own **model database** holding everything that makes up that deployment. This split is the key structural fact of the Juju data model; everything else follows from it.

(arch-datamodel-controller)=
### The controller database

The controller database holds the entities that are shared across all models or that govern access to the controller itself. There is exactly one controller database per controller.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart LR
    subgraph subController["Controller database"]
        USER
        CLOUD
        CONTROLLER
        CREDENTIAL
        MODEL
        SECRET-BACKEND["SECRET BACKEND"]
        EXTERNAL-CONTROLLER["EXTERNAL CONTROLLER"]
        USER -->|owns| CREDENTIAL
        USER -->|accesses| CLOUD
        USER -->|accesses| CONTROLLER
        USER -->|accesses| MODEL
        CLOUD -->|defines| CREDENTIAL
        CLOUD -->|hosts| CONTROLLER
        CONTROLLER -->|stores| CREDENTIAL
        CONTROLLER -->|stores| SECRET-BACKEND
        MODEL -->|"deployed on"| CLOUD
        MODEL -->|uses| CREDENTIAL
        MODEL -->|"backed by"| SECRET-BACKEND
        MODEL -->|"may reference"| EXTERNAL-CONTROLLER
    end
```
*The controller database entities and their relationships. The model record lives here with foreign keys to cloud, credential, and secret backend; it points to the model's own Dqlite database via its namespace.*

#### Cloud

A cloud is an infrastructure provider -- a public cloud (AWS, Azure, GCP), a private cloud (OpenStack, MAAS), or a local substrate (LXD, MicroK8s). It plays two roles in Juju: it is the substrate the controller itself runs on, and it is the source of machines and pods that models provision for their applications. A controller can manage models on multiple clouds. Juju records each cloud's endpoint, supported authentication types, regions, and CA certificates.

```{ibnote}
See more: {ref}`cloud`
```

#### Credential

A credential is a set of authentication attributes -- an API key, certificate, username/password, or similar -- that authorises Juju to call a cloud provider's API on your behalf. A credential belongs to exactly one cloud and is owned by exactly one user. A model optionally references one credential; when set, that credential is what Juju uses to provision and manage machines on that cloud. A model with no credential set relies on the cloud's ambient authentication (for example, an instance role).

```{ibnote}
See more: {ref}`credential`
```

#### Controller

A controller is the Juju management node. There is exactly one controller record per controller database. In high-availability mode the controller runs across multiple machines -- each is a `controller_node` record -- but they all share one controller identity and one Dqlite cluster. The controller runs on a cloud and provisions resources from clouds on behalf of the models it manages.

```{ibnote}
See more: {ref}`controller`
```

#### User

A user is a Juju identity that can authenticate to a controller. Users are granted permissions on clouds, controllers, models, and offers (published endpoints for cross-model integration -- covered in the model database section). The available access levels differ by object type: `add-model` and `admin` on a cloud, `login` and `superuser` on a controller, and `read`, `write`, or `admin` on a model. The user who bootstrapped the controller is the initial superuser.

```{ibnote}
See more: {ref}`user`, {ref}`user-access-levels`
```

#### Model

A model is a named deployment environment: it contains the applications, machines, relations, storage, and networks that together deliver a service. The model record lives in the controller database, with references to the cloud it runs on, an optional credential, and an optional region. It also points to its own Dqlite database -- the model database -- where the actual deployment entities live. A model is either an IAAS model (running on a machine cloud such as AWS or MAAS) or a CAAS model (running on a Kubernetes cluster). One model, named `controller`, is reserved for the controller itself; all others are user-created workload models.

```{ibnote}
See more: {ref}`model`
```

#### SSH keys

A user's public SSH keys are stored in the controller database (`user_public_ssh_key`). Keys are then selectively authorised on models via `model_authorized_keys`, which links a model to specific user keys. This is what populates the `~/.ssh/authorized_keys` on provisioned machines, allowing `juju ssh` to work.

```{ibnote}
See more: {ref}`manage-ssh-keys`
```

#### Secret backend

A secret backend is an external store for secret content -- Vault, a Kubernetes secrets store, or the controller's own internal store. Secret backend records live in the controller database (`secret_backend`). Each model is associated with exactly one secret backend via `model_secret_backend`; secret revisions stored in an external backend are tracked by `secret_backend_reference`.

```{ibnote}
See more: {ref}`secret`
```

#### External controller

An external controller record stores the API addresses and CA certificate of a remote Juju controller. It is created when a cross-model integration -- where one model consumes an endpoint published by an application in a different model -- targets an offer on a controller other than the local one. This record is what allows the local controller to authenticate and connect to the remote one.

```{ibnote}
See more: {ref}`cross-model-integration`
```

(arch-datamodel-model)=
### The model database

Each model has its own Dqlite database. Model membership is implicit -- records belong to a model by virtue of which database they live in, not by a column in the table. The model database holds everything that makes up a deployment, organised here into four clusters.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart LR
    subgraph subDeploy["Deployment cluster"]
        APPLICATION -->|"consists of"| UNIT
        APPLICATION -->|"deployed from"| CHARM
        APPLICATION -->|"bound to"| SPACE
        APPLICATION -->|has| CONSTRAINT
        UNIT -->|"runs on"| MACHINE
        MACHINE -->|has| CONSTRAINT
    end

    subgraph subIntegration["Integration cluster"]
        RELATION -->|connects| APPLICATION2[APPLICATION]
        APPLICATION2 -->|publishes| OFFER
        RELATION -->|"via"| OFFER
    end

    subgraph subRuntime["Runtime cluster"]
        OPERATION -->|targets| UNIT2[UNIT]
        UNIT2 -->|owns| STORAGE
        UNIT2 -->|"owns or consumes"| SECRET
        UNIT2 -->|uses| RESOURCE
    end

    subgraph subNetwork["Network cluster"]
        SPACE2[SPACE] -->|contains| SUBNET
        UNIT3[UNIT] -->|"has"| PORT-RANGE["PORT RANGE"]
    end
```
*The model database organised into clusters. APPLICATION, UNIT, and MACHINE form the deployment core. RELATION and OFFER handle integration within and across models. OPERATION, STORAGE, SECRET, and RESOURCE are runtime records produced as the deployment runs. SPACE, SUBNET, and PORT RANGE describe the network configuration.*

#### Deployment cluster

**Application** -- A running instance of a **charm** (the software package that tells Juju how to operate an application) inside a model. An application consists of one or more units and is always deployed from a specific charm revision. Every application is bound to a network space by default.

**Unit** -- A single running instance of the software an application describes. Runs on a machine (or pod on Kubernetes). Named on the pattern `<application>/<unit ID>` -- for example, `mysql/0`. During a rolling upgrade a unit may temporarily run a different charm revision from its application.

**Machine** -- What a unit runs on. On machine clouds it is a VM or bare-metal host; on Kubernetes it is the pod hosting the unit. LXD system containers are also represented as machines: a container on machine `0` appears as `0/lxd/0` and has a `machine_parent` record pointing to its host machine.

**Constraint** -- A set of hardware or placement requirements (`constraint` table) referenced by applications and machines. Attributes include architecture, CPU, memory, root disk, instance type, virt type, spaces, zones, and tags. Application constraints are passed to the cloud provider when provisioning machines for units.

```{ibnote}
See more: {ref}`application`, {ref}`unit`, {ref}`machine`, {ref}`constraint`
```

#### Integration cluster

**Relation** -- Connects two applications so they can exchange data via relation data bags. A relation links to applications via `relation_endpoint → application_endpoint → application`. Relations can also cross model boundaries.

**Offer** -- A named set of application endpoints published for cross-model consumption (`offer`, `offer_endpoint`). When a consumer integrates with an offer, an `offer_connection` record is created on the offering side and a synthetic remote application (`application_remote_offerer`) is created on the consuming side.

```{ibnote}
See more: {ref}`relation`, {ref}`cross-model-integration`
```

#### Runtime cluster

**Operation** -- A runtime invocation of an action against one or more units (`operation`, `operation_task`, `operation_unit_task`). Created when a user runs `juju run`. Stores output, logs, and exit status per task.

**Storage** -- A volume or filesystem attached to a unit. The charm declares storage needs (`charm_storage`); at deploy time those declarations are resolved against a storage pool to produce `storage_instance` records owned by a unit and attached via `storage_attachment`.

**Secret** -- A versioned, encrypted value. Has an owner (application, unit, or model) and is granted to consumers with a role of view or manage. Content is stored inline or delegated to the model's secret backend.

**Resource** -- A runtime record of a versioned binary or file blob (`resource`, `application_resource`, `unit_resource`). Runtime instance of a declared charm resource. Can be refreshed independently of the charm revision.

```{ibnote}
See more: {ref}`action`, {ref}`storage`, {ref}`secret`, {ref}`resource`
```

#### Network cluster

**Space** -- A named network segment. Every application has a mandatory default space binding. Individual endpoints can be bound to different spaces. Spaces constrain where units are placed.

**Subnet** -- A CIDR range belonging to a space. Units receive IP addresses from subnets.

**Port range** -- A protocol and port range that a unit exposes (`port_range`), optionally scoped to a relation endpoint. Opened and closed by the charm via hook commands. Used to configure cloud firewall or security-group rules.

```{ibnote}
See more: {ref}`space`, {ref}`expose-an-application`
```

(arch-datamodel-charm)=
### Charm declarations

A charm is the software package that tells Juju how to install, configure, scale, and operate an application. It is a ZIP archive containing metadata and operator code. When fetched -- from Charmhub or a local path -- the controller stores a `charm` record in the model database together with four sets of declarations that describe the charm's contract:

- **Integrations** (`charm_relation`) -- the relation endpoints the charm exposes: name, role (provider/requirer/peer), interface, and scope.
- **Actions** (`charm_action`) -- named operations a user can invoke against a unit: name, description, and parameter schema.
- **Configuration schema** (`charm_config`) -- the config keys the charm accepts, with types and default values. The *schema*; the live per-application values are stored separately in `application_config`.
- **Storage specifications** (`charm_storage`) -- the storage mounts the charm declares, with kind (block or filesystem), minimum size, and cardinality.
- **Resources** (`charm_resource`) -- binary or file blobs (OCI images, tarballs) declared by the charm. The *declaration*; the downloaded blobs are stored in runtime `resource` records.

Charmhub is an external registry -- nothing about Charmhub is stored in Juju's databases. What the controller *does* store is the charm's origin: `charm_download_info` records the Charmhub identifier, download URL, and size so the deployment is reproducible and `juju refresh` can locate the newer revision.

```{ibnote}
See more: {ref}`charm`, {ref}`charmhub`
```

#### Status and configuration

Two cross-cutting concerns apply to entities across the model database:

**Status** -- Every application, unit, machine, and model has a status record (separate tables per entity type). Application and unit status is set by a charm via `status-set` and reflects the charm's view of the workload. Machine and model status is derived by the controller from the states of the entities they contain.

**Configuration** -- Applications have live configuration (`application_config`) -- key-value settings a user provides that the charm reads during hooks. Models have model configuration (`model_config`) -- controller-level settings that govern model-wide behaviour such as logging level, update intervals, and image streams.

```{ibnote}
See more: {ref}`status`, {ref}`configuration`
```

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

In a running deployment the programs run on infrastructure in a characteristic arrangement. Here is what that looks like.

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
- **Controller to agents** -- Agents use an event-driven contract built on **watchers**: long-lived API calls that block until a change relevant to that agent occurs, then return a summary of what changed. How watchers drive reconciliation is described in {ref}`the operations section <arch-reconciliation-contract>`.
- **Unit agent to charm** -- in two directions: downward (the agent sets environment variables and runs the charm's `dispatch` script as a subprocess), and upward (during a hook the charm calls {ref}`hook commands <hook-command>` -- the `jujuc` binaries), over a Unix socket the unit agent listens on.
- **Charm to workload** -- On Kubernetes, through the **Pebble API**, an HTTP API served by Pebble inside each workload container. On machine clouds, the charm drives its workload directly using standard operating-system mechanisms, since the charm and workload are co-located.

```{ibnote}
See more: {ref}`jujuc`, {ref}`pebble`, {ref}`database`
```

(arch-operations)=
## Operations

The data model and software sections show what a deployment is: the state it keeps and the programs that act on it. This section shows how state transitions happen over time: the ideas that make Juju converge, the operations that carry a deployment through its life, and at close range, how a single unit runs its charm.

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

Deploying an application adds its software to the model and arranges for it to run on a resource. The controller writes an application record (with its units) to the database, a model agent asks the cloud for resources (a VM or pod), and once the resource is ready the controller starts the unit agent, which runs the install sequence.

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

:::::

```{ibnote}
See more: {ref}`command-juju-deploy`
```

#### Integrate

Integrating connects one application to another so they can exchange data. When you integrate two applications, the controller writes a relation record; the unit agents on both sides are notified via their watchers and each runs the relation hooks in sequence.

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
*Integrating two applications. The controller writes the relation record, which fires watchers on both unit agents. Each runs `relation-created` then `relation-joined`, followed immediately by `relation-changed`. Thereafter, each time a unit writes its relation data bag, the other unit receives a further `relation-changed`.*

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

Two levels have non-obvious internal sequences worth understanding for troubleshooting: unit removal and model removal.

##### Unit removal

Unit removal is driven by the unit agent itself once the controller marks the unit Dying. The agent runs cleanup hooks in the order prescribed by the teardown phase before the unit can be declared Dead. A hook failure at any stage blocks the unit in its current state until the failure is resolved or the removal is forced.

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
*Unit removal. Once the controller marks the unit Dying, the unit agent runs the teardown hooks: `stop` first, then `storage-detaching` and `relation-broken` (each preceded by `relation-departed` for every known remote unit) in any order, then `remove` last. The unit is then marked Dead and the controller releases the machine if no longer needed.*

```{note}
A hook failure at any stage leaves the unit in `error` state and blocks further progress. Use `juju resolved` to retry or skip the failed hook. Use `juju remove-unit --force` to bypass hooks entirely -- but note that forced removal may leave orphaned relation data or unreleased storage on the cloud side.
```

##### Model removal

Model removal is orchestrated by the undertaker worker, which watches for models that have been set Dying and drives them through to deletion of both the model records and the model's Dqlite database.

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
*Model removal. The undertaker worker watches for the model to be set Dying, destroys all applications (each unit going through its own removal sequence), waits for all machines to be released, then deletes the model records and its Dqlite database.*

```{note}
A `juju destroy-model` that hangs is almost always a unit stuck in its removal hook sequence. Use `juju status` on the dying model to identify the blocked unit, then `juju resolved` or `juju remove-unit --force` to unblock it.
```

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

```{mermaid}
flowchart LR
    W["Wait\n(watcher)"]
    S["Snapshot\n(remote state)"]
    R["Resolve\n(next hook)"]
    D["Dispatch\n(run charm)"]
    C["Commit\n(flush writes)"]

    W --> S --> R --> D --> C --> W
```
*The unit agent's control loop: wait for a watcher to fire, snapshot remote state, resolve the next hook, dispatch it, commit buffered writes, and repeat.*

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

The controller database is the single source of truth for Juju state. When a hook runs, the charm has no in-process memory of previous runs: everything it needs comes from the database, via hook commands, on demand.

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
