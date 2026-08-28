---
myst:
  html_meta:
    description: "Juju architecture explained: the state Juju stores, the software that acts on it, and the operations that drive a deployment through its life."
---

(juju-architecture)=
# Juju architecture

The fundamental problem Juju solves: a model admin wants to deploy and operate applications -- each needing compute, storage, and networking -- on any cloud, without writing cloud-specific or application-specific glue code every time. In production, applications never run in isolation: a workload needs to integrate with observability, identity, secret management, databases, and more, and the whole system needs to keep working through upgrades, scaling events, and migrations.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    ADMIN(("user"))
    APP1["<b>application 1</b><br/><small>cloud1/app1</small>"]
    APP2["<b>application 2</b><br/><small>cloud1/app2</small>"]
    APP3["<b>application 3</b><br/><small>cloud2/app1</small>"]

    ADMIN -->|"operates"| APP1
    ADMIN -->|"operates"| APP2
    ADMIN -->|"operates"| APP3

    style ADMIN fill:#F5F0E8,stroke:#999,color:#000
    style APP1 fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style APP2 fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style APP3 fill:#4A90D9,stroke:#2C6FAC,color:#FFF
```
*The problem: operate a system of applications across any cloud -- deploy, configure, integrate, scale, upgrade, remove.*

Juju sits between the user and the clouds. The user declares what they want; Juju stores that declaration and drives the real world toward it. Juju runs as a **controller** -- a process the user talks to through a **client** (the `juju` CLI, the Terraform provider, or JAAS). The controller talks to clouds to request hosts, and to **Charmhub** to fetch **charms** -- software packages that encode how to deploy, configure, integrate, upgrade, and remove each application. Crucially, in Juju all integration between applications goes *through the controller* in a star topology -- there are no direct application-to-application connections.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    ADMIN(("user"))
    CLIENT["<b>client</b>"]
    CONTROLLER["<b>controller</b><br/><small>C1</small>"]
    subgraph apps[" "]
        direction TB
        APP_1["<b>charmed application 1</b><br/><small>C1/c1/m1/a1</small>"]
        APP_2["<b>charmed application 2</b><br/><small>C1/c1/m1/a2</small>"]
        APP_3["<b>charmed application 3</b><br/><small>C1/c2/m1/a1</small>"]
    end

    ADMIN -->|"sends commands to"| CLIENT
    CLIENT -->|"calls Juju API on"| CONTROLLER
    CONTROLLER -->|"operates"| APP_1
    CONTROLLER -->|"operates"| APP_2
    CONTROLLER -->|"operates"| APP_3

    style ADMIN fill:#F5F0E8,stroke:#999,color:#000
    style CLIENT fill:#E95420,stroke:#C74210,color:#FFF
    style CONTROLLER fill:#E95420,stroke:#C74210,color:#FFF
    style APP_1 fill:#4A90D9,stroke:#E95420,stroke-width:2px,color:#FFF
    style APP_2 fill:#4A90D9,stroke:#E95420,stroke-width:2px,color:#FFF
    style APP_3 fill:#4A90D9,stroke:#E95420,stroke-width:2px,color:#FFF
    style apps fill:none,stroke:none
```
*Juju enters. Controller C1 sits in the middle: it manages two models on different clouds, fetches charms from Charmhub, and operates all charmed applications -- shown with orange borders. Each app's annotation shows its address in the hierarchy: `C1/c1/m1/a1` means controller C1, cloud c1, model m1, application a1. A cloud is registered on a controller, not exclusively owned -- the same cloud can be registered on multiple controllers.*

Inside the controller is a **Dqlite** database cluster: a **controller-db** holding shared state (clouds, credentials, users, model records), and one database per model -- **model1-db**, **model2-db**, and so on -- holding the deployment entities for that model. On each host the cloud provisions, Juju runs a **unit agent** alongside the workload; the unit agent runs the charm, which operates the application.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    ADMIN(("user"))
    CLIENT["<b>client</b>"]
    CONTROLLER["<b>controller</b><br/><small>C1</small>"]


    subgraph apps[" "]
        direction TB
        subgraph unit1["C1/c1/m1/a1/0"]
            direction TB
            AGENT_1["<b>unit agent</b>"]
            CHARM_1["<b>charm</b>"]
            WORKLOAD_1["<b>workload</b>"]
            AGENT_1 -->|"runs"| CHARM_1
            CHARM_1 -->|"operates"| WORKLOAD_1
        end
        subgraph unit2["C1/c1/m1/a2/0"]
            direction TB
            AGENT_2["<b>unit agent</b>"]
            CHARM_2["<b>charm</b>"]
            WORKLOAD_2["<b>workload</b>"]
            AGENT_2 -->|"runs"| CHARM_2
            CHARM_2 -->|"operates"| WORKLOAD_2
        end
        subgraph unit3["C1/c2/m1/a1/0"]
            direction TB
            AGENT_3["<b>unit agent</b>"]
            CHARM_3["<b>charm</b>"]
            WORKLOAD_3["<b>workload</b>"]
            AGENT_3 -->|"runs"| CHARM_3
            CHARM_3 -->|"operates"| WORKLOAD_3
        end
    end

    ADMIN -->|"sends commands to"| CLIENT
    CLIENT -->|"calls Juju API on"| CONTROLLER
    CONTROLLER -->|"drives"| AGENT_1
    CONTROLLER -->|"drives"| AGENT_2
    CONTROLLER -->|"drives"| AGENT_3

    style ADMIN fill:#F5F0E8,stroke:#999,color:#000
    style CLIENT fill:#E95420,stroke:#C74210,color:#FFF
    style CONTROLLER fill:#E95420,stroke:#C74210,color:#FFF
    style AGENT_1 fill:#E95420,stroke:#C74210,color:#FFF
    style AGENT_2 fill:#E95420,stroke:#C74210,color:#FFF
    style AGENT_3 fill:#E95420,stroke:#C74210,color:#FFF
    style CHARM_1 fill:#FFF,stroke:#E95420,color:#000
    style CHARM_2 fill:#FFF,stroke:#E95420,color:#000
    style CHARM_3 fill:#FFF,stroke:#E95420,color:#000
    style WORKLOAD_1 fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style WORKLOAD_2 fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style WORKLOAD_3 fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style unit1 fill:none,stroke:#AAA
    style unit2 fill:none,stroke:#AAA
    style unit3 fill:none,stroke:#AAA
    style apps fill:none,stroke:none
```
*Juju unpacked. Three units, each a grey box containing unit agent → charm → workload. The subgraph label is the unit's full address in the hierarchy: `C1/c1/m1/a1/0` means controller C1, cloud c1, model m1, application a1, unit 0. Applications 1↔2 and 2↔3 are integrated -- all routed through the controller. Color key: orange fill = Juju software (this repo); white with orange border = Juju ecosystem (charm); blue fill = your workload.*

The three sections below zoom in on different aspects of this picture:

1. {ref}`The data model <arch-datamodel>` -- what lives in the controller DB and model DBs.
2. {ref}`The software <arch-software>` -- the programs, their topology, and how they communicate.
3. {ref}`The operations <arch-operations>` -- how state changes over time: bootstrap, deploy, integrate, remove.

(arch-datamodel)=
## The data model

The controller stores its state in two databases using **Dqlite**, a distributed SQLite implementation embedded in the controller process. The **controller database** holds entities shared across all models: clouds, credentials, users, and the model registry itself. Each model has its own **model database** holding everything that makes up that deployment. This split is the key structural fact of the Juju data model; everything else follows from it.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart TB
    %% Controller DB spine
    USER --> CONTROLLER
    USER --> CLOUD
    USER --> CREDENTIAL
    CLOUD --> CONTROLLER
    CREDENTIAL --> CONTROLLER
    CLOUD --> CREDENTIAL
    SECRET_BACKEND["SECRET BACKEND"] --> CONTROLLER
    CONTROLLER --> MODEL

    %% Model DB spine
    MODEL --> APPLICATION
    APPLICATION --> UNIT
    UNIT --> MACHINE

    %% References hanging off APPLICATION
    APPLICATION --> CHARM
    APPLICATION --> RELATION
    APPLICATION --> OFFER

    %% Runtime records hanging off UNIT
    UNIT --> STORAGE
    UNIT --> SECRET
    UNIT --> RESOURCE
    UNIT --> PORT_RANGE["PORT RANGE"]

    %% Network
    APPLICATION --> SPACE
    SPACE --> SUBNET

    %% Cross-cutting (dashed)
    CONSTRAINT -. "constrains" .-> APPLICATION
    CONSTRAINT -. "constrains" .-> MACHINE
    STATUS -. "on" .-> APPLICATION
    STATUS -. "on" .-> UNIT
    STATUS -. "on" .-> MACHINE
    STATUS -. "on" .-> MODEL
    CONFIGURATION -. "on" .-> APPLICATION
    CONFIGURATION -. "on" .-> MODEL
    EXTERNAL_CONTROLLER["EXTERNAL CONTROLLER"] -. "referenced by" .-> MODEL

    style USER fill:#E95420,stroke:#C74210,color:#FFF
    style CONTROLLER fill:#E95420,stroke:#C74210,color:#FFF
    style CLOUD fill:#E95420,stroke:#C74210,color:#FFF
    style CREDENTIAL fill:#E95420,stroke:#C74210,color:#FFF
    style SECRET_BACKEND fill:#E95420,stroke:#C74210,color:#FFF
    style EXTERNAL_CONTROLLER fill:#E95420,stroke:#C74210,color:#FFF
    style MODEL fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style APPLICATION fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style UNIT fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style MACHINE fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style CHARM fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style RELATION fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style OFFER fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style STORAGE fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style SECRET fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style RESOURCE fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style PORT_RANGE fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style SPACE fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style SUBNET fill:#4A90D9,stroke:#2C6FAC,color:#FFF
    style STATUS fill:#888,stroke:#666,color:#FFF
    style CONSTRAINT fill:#888,stroke:#666,color:#FFF
    style CONFIGURATION fill:#888,stroke:#666,color:#FFF
```
*The Juju entity graph. Orange = controller database; blue = model database; grey =
cross-cutting attributes. The spine runs USER → CONTROLLER → MODEL → APPLICATION →
UNIT → MACHINE. Everything else hangs off that spine.*

(arch-datamodel-controller)=
### The controller database

The controller database holds the entities that are shared across all models or that govern access to the controller itself. There is exactly one controller database per controller.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart TB
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

A controller is the Juju management node. There is exactly one controller record per controller database. In high-availability mode the controller runs across multiple machines -- each is a `controller_node` record -- but they all share one controller identity and one Dqlite cluster. The controller runs on a cloud host and requests machines or pods from clouds on behalf of the models it manages.

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
See more: {ref}`manage-offers`
```

(arch-datamodel-model)=
### The model database

Each model has its own Dqlite database. Model membership is implicit -- records belong to a model by virtue of which database they live in, not by a column in the table. The model database holds everything that makes up a deployment, organised here into four clusters.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": false}} }%%
flowchart TB
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
See more: {ref}`relation`, {ref}`manage-offers`
```

#### Runtime cluster

**Operation** -- A runtime invocation of an action against one or more units (`operation`, `operation_task`, `operation_unit_task`). Created when a user runs `juju run`. Stores output, logs, and exit status per task.

**Storage** -- A volume or filesystem attached to a unit. The charm declares storage needs (`charm_storage`); at deploy time those declarations are resolved against a storage pool to produce `storage_instance` records owned by a unit and attached via `storage_attachment`.

**Secret** -- A versioned, encrypted value. Has an owner (application, unit, or model) and is granted to consumers with a role of view or manage. Content is stored inline or delegated to the model's secret backend.

**Resource** -- A runtime record of a versioned binary or file blob (`resource`, `application_resource`, `unit_resource`). Runtime instance of a declared charm resource. Can be refreshed independently of the charm revision.

```{ibnote}
See more: {ref}`action`, {ref}`storage`, {ref}`secret`, {ref}`charm-resource`
```

#### Network cluster

**Space** -- A named network segment. Every application has a mandatory default space binding. Individual endpoints can be bound to different spaces. Spaces constrain where units are placed.

**Subnet** -- A CIDR range belonging to a space. Units receive IP addresses from subnets.

**Port range** -- A protocol and port range that a unit exposes (`port_range`), optionally scoped to a relation endpoint. Opened and closed by the charm via hook commands. Used to configure cloud firewall or security-group rules.

```{ibnote}
See more: {ref}`space`, {ref}`command-juju-expose`
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
See more: {ref}`charm`, [Charmhub](https://charmhub.io)
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
flowchart TB
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
flowchart TB
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


### How the controller is deployed

The controller is itself a deployed unit -- but only in structure, not in how it is reconciled. On Kubernetes, the controller runs as the `juju-controller` application with a single unit (`controller-0`) in a pod, with a `charm` container and an `api-server` workload container (with Pebble inside it supervising the `jujud` controller-agent service). On a machine cloud it is a unit on the controller machine. Its charm, `juju-controller`, is a real charm record -- you can see it and upgrade it like any charm.

But the controller is not operated the way application charms are. For an ordinary unit, reconciliation means the unit agent reconciles the model by dispatching the charm's hooks. For the controller, the charm is recorded but not dispatched to operate the controller: what runs the controller is the process itself, whose manifold tree serves the API, runs the model workers, and holds the in-process Dqlite store. The `application_controller` marker flags the controller application as special and non-ordinary -- it exists so the controller is not treated as a workload charm you can freely integrate and drive via hooks.

So the controller is a unit in form, but the loop that keeps its deployments true is jujud's own machinery, not the charm loop -- the one place in the system where the unit model is asymmetric.

This is why the topology diagrams above draw the controller pod or machine as `jujud` hosting the controller and model agent workers with Dqlite in-process: the box is the controller unit's workload and its reconciling machinery together, and the charm marked on it does not drive that machinery.

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

A key architectural point: in Juju, applications never communicate directly with each other. All integration is mediated by the controller in a **star topology** -- every arrow in the sequence diagram below goes from a unit agent to the controller or back, never from one unit agent to the other. The controller holds the relation record and the data bags; each unit reads and writes its side through the controller. This means the controller is always the single source of truth for what two applications have agreed upon.

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
*Integrating two applications. Every arrow passes through the controller -- UA1 and UA2 never communicate directly. The controller holds the relation record; each unit writes its data bag to the controller, which notifies the other via a watcher. This is the star topology in practice.*

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
flowchart TB
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

(arch-entity-diagrams)=
## Entity relationship diagrams

The sections below show the Juju data model one dimension at a time. Each diagram
isolates a single concern so it stays readable. All diagrams show records in the
Juju databases -- not running processes or files on disk. Together they give a
complete picture of what the controller stores.

### Scope and containment

What contains what. This diagram answers: if you remove or inspect entity A,
what else falls within its scope? A controller is the scope of its models; a
model is the scope of its applications; an application is the scope of its
units. The diagram also shows what a unit directly owns at runtime.

Note that a cloud can be known at three levels independently: as a local record
in the client's own configuration file (`~/.local/share/juju/clouds.yaml`),
as a registered record in the controller database, and as a denormalised
reference mirrored into each model database so model workers can drive
provisioning without crossing databases. The diagram shows the controller
database view: a credential is owned by a user and scoped to a cloud; a model
references a cloud (and optionally a region and credential).

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    USER -->|"owns"| CREDENTIAL["CREDENTIAL"]
    CREDENTIAL -->|"is scoped to"| CLOUD
    USER -->|"is granted access on"| CLOUD
    CONTROLLER -->|"manages"| MODEL
    MODEL -->|"references"| CLOUD
    MODEL -->|"contains"| APPLICATION
    APPLICATION -->|"consists of"| UNIT
    UNIT -->|"runs on"| MACHINE
    APPLICATION -->|"is deployed from"| CHARM
    UNIT -->|"runs revision of"| CHARM
    APPLICATION -->|"exposes"| OFFER
    RELATION -->|"connects"| APPLICATION
    UNIT -->|"participates in"| RELATION
    UNIT -->|"owns"| STORAGE
    UNIT -->|"owns or consumes"| SECRET
    UNIT -->|"uses"| RESOURCE
    UNIT -->|"declares"| PORT_RANGE["PORT RANGE"]
    APPLICATION -->|"is bound to"| SPACE
    SPACE -->|"contains"| SUBNET
```
*Both APPLICATION and UNIT reference CHARM directly and independently -- during
a rolling upgrade they can point to different revisions.*

### Lifecycle

The alive / dying / dead states apply to MODEL, APPLICATION, UNIT, MACHINE, and
RELATION. These states are what you see in `juju status` when a removal is in
progress. A unit stuck in dying (because a hook failed) is why `juju resolved`
and `juju remove-unit --force` exist.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
stateDiagram-v2
    direction LR
    [*] --> alive
    alive --> dying : remove requested<br/>(controller sets)
    dying --> dead : teardown complete<br/>(agent sets)
    dead --> [*]
```
*A unit or machine appears as "dying" in `juju status` while its teardown hooks
are running. It moves to dead once the agent confirms completion, after which
the controller deletes its records.*

### Access control

Permissions are a separate dimension from ownership. A user is granted an
access level on a specific object. The valid combinations are:

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart LR
    USER -->|"add-model, admin"| CLOUD
    USER -->|"login, superuser"| CONTROLLER
    USER -->|"read, write, admin"| MODEL
    USER -->|"read, consume, admin"| OFFER
```
*A user can hold at most one access level per object instance. Access levels
are cumulative: admin implies write implies read.*

### Cloud as provisioning target

A cloud in Juju is any infrastructure provider that exposes an API for compute,
storage, and networking -- a public cloud (AWS, GCP, Azure), a private cloud
(OpenStack, MAAS), or a local substrate (LXD, MicroK8s, Kubernetes). The model
database holds a read-only mirror of the cloud name and type so model workers
can drive provisioning locally without crossing to the controller database.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    CLOUD -->|"has"| CLOUD_REGION["CLOUD REGION"]
    CLOUD -->|"supports"| AUTH_TYPE["AUTH TYPE"]
    CREDENTIAL -->|"authenticates against"| CLOUD
    MODEL -->|"provisions resources from"| CLOUD
    MODEL -->|"optionally targets"| CLOUD_REGION
    MODEL -->|"uses"| CREDENTIAL
    MACHINE_CLOUD_INSTANCE["MACHINE CLOUD INSTANCE"] -->|"is the cloud record of"| MACHINE
```
*The cloud record stores the provider endpoint, supported auth types, regions,
and CA certificates. MACHINE CLOUD INSTANCE is what the cloud provider reports
back once a machine is provisioned -- instance ID, architecture, CPU, memory.*

### Placement and infrastructure binding

How logical records are associated with physical infrastructure. On machine
clouds, a unit and its machine share the same network node -- that shared
record is how Juju associates a logical unit with its physical host. IP
addresses belong to the machine's network interfaces, not to the unit directly.
On Kubernetes there are no machine records; the unit is associated with a pod.

:::::{tab-set}

::::{tab-item} Machine clouds

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    UNIT -->|"shares network node with"| NET_NODE["NET NODE"]
    MACHINE -->|"shares network node with"| NET_NODE
    NET_NODE -->|"has"| LINK_LAYER_DEVICE["LINK LAYER DEVICE"]
    LINK_LAYER_DEVICE -->|"has"| IP_ADDRESS["IP ADDRESS"]
    IP_ADDRESS -->|"belongs to"| SUBNET
    SUBNET -->|"belongs to"| SPACE
    MACHINE_PARENT["MACHINE PARENT"] -->|"records container nesting of"| MACHINE
```
*The shared network node is the join between the logical unit record and the
physical machine record. LXD containers are nested machines; the machine_parent
record tracks which machine a container lives on.*

::::

::::{tab-item} Kubernetes

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    UNIT -->|"shares network node with"| NET_NODE["NET NODE"]
    K8S_POD["K8S POD"] -->|"belongs to"| UNIT
    K8S_POD -->|"shares network node with"| NET_NODE
    APPLICATION -->|"has"| K8S_SERVICE["K8S SERVICE"]
    K8S_SERVICE -->|"shares network node with"| NET_NODE
```
*On Kubernetes there are no machine records. Each unit has a pod record. The
k8s_service record tracks the stable cluster-IP endpoint for the application.*

::::

:::::

### Charm record and its declarations

A charm record in the database represents a specific revision of a charm -- its
source, revision number, and archive hash -- plus the set of declarations the
charm makes: what endpoints it exposes, what config keys it accepts, what
storage it needs, what resources it bundles, and what actions it supports.
Both APPLICATION and UNIT reference the charm record directly, and
independently -- during a rolling upgrade they can point to different revisions.
The application holds the live configuration values; the charm record holds
only the schema.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    CHARM -->|"declares"| CHARM_RELATION["CHARM RELATION (endpoint)"]
    CHARM -->|"declares"| CHARM_ACTION["CHARM ACTION"]
    CHARM -->|"declares"| CHARM_CONFIG["CHARM CONFIG (schema)"]
    CHARM -->|"declares"| CHARM_STORAGE["CHARM STORAGE (spec)"]
    CHARM -->|"declares"| CHARM_RESOURCE["CHARM RESOURCE (declaration)"]

    APPLICATION -->|"is deployed from"| CHARM
    APPLICATION -->|"has live values in"| APPLICATION_CONFIG["APPLICATION CONFIG"]
    APPLICATION -->|"has"| APPLICATION_ENDPOINT["APPLICATION ENDPOINT"]
    APPLICATION_ENDPOINT -->|"binds"| CHARM_RELATION
    APPLICATION_ENDPOINT -->|"is bound to"| SPACE

    UNIT -->|"runs revision of"| CHARM
    UNIT -->|"owns"| STORAGE
    UNIT -->|"uses"| RESOURCE
    UNIT -->|"declares"| PORT_RANGE["PORT RANGE"]
```
*The charm record is the template; APPLICATION and UNIT are instances of it.
The charm config record is the schema; application_config holds the live values
set by the user. An application endpoint binds a charm relation declaration to
a network space.*

### Status

Status is a time-varying property recorded per entity. Different actors write
different status records: the charm writes workload status via the `status-set`
hook command; the agent writes its own agent status; the controller derives
machine and model status from the states of the entities they contain.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart LR
    CHARM -->|"writes via status-set"| UNIT_WORKLOAD_STATUS["UNIT WORKLOAD STATUS"]
    AGENT -->|"writes"| UNIT_AGENT_STATUS["UNIT AGENT STATUS"]
    AGENT -->|"writes"| K8S_POD_STATUS["K8S POD STATUS"]
    CHARM -->|"writes via status-set"| APPLICATION_STATUS["APPLICATION STATUS"]
    CONTROLLER -->|"derives"| MACHINE_STATUS["MACHINE STATUS"]
    CONTROLLER -->|"derives"| MODEL_STATUS["MODEL STATUS"]

    UNIT_WORKLOAD_STATUS -->|"tracks status of"| UNIT
    UNIT_AGENT_STATUS -->|"tracks status of"| UNIT
    K8S_POD_STATUS -->|"tracks status of"| UNIT
    APPLICATION_STATUS -->|"tracks status of"| APPLICATION
    MACHINE_STATUS -->|"tracks status of"| MACHINE
    MODEL_STATUS -->|"tracks status of"| MODEL
```
*Unit status has two independent records: agent status (is the agent running and
healthy?) and workload status (is the application the charm manages healthy?).
On Kubernetes there is a third: the pod status reported by the cluster.*

### Relations and databags

How integration between applications is structured in the data model. All
relation data flows through the controller -- there are no direct
application-to-application connections. An application endpoint is the binding
of a charm relation declaration to a space; a relation endpoint is the record
that links a live relation to one of those application endpoints. Databags exist
at two levels: one per participating application (written by the application
leader) and one per participating unit.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    APPLICATION -->|"has"| APPLICATION_ENDPOINT["APPLICATION ENDPOINT"]
    APPLICATION_ENDPOINT -->|"binds"| CHARM_RELATION["CHARM RELATION"]
    RELATION -->|"is joined by"| RELATION_ENDPOINT["RELATION ENDPOINT"]
    RELATION_ENDPOINT -->|"references"| APPLICATION_ENDPOINT
    RELATION_ENDPOINT -->|"has"| RELATION_APP_DATABAG["APPLICATION DATABAG"]
    RELATION_ENDPOINT -->|"has"| RELATION_UNIT["RELATION UNIT"]
    RELATION_UNIT -->|"references"| UNIT
    RELATION_UNIT -->|"has"| RELATION_UNIT_DATABAG["UNIT DATABAG"]

    APPLICATION -->|"publishes"| OFFER
    OFFER -->|"exposes"| APPLICATION_ENDPOINT
```
*A relation connects two application endpoints (one per participating
application, or one for a peer relation). Each application endpoint contributes
one application-level databag (written by the leader) and one unit-level
databag per participating unit. An offer publishes application endpoints for
cross-model consumption.*


### Secrets

A secret is a versioned sensitive value -- a password, API key, certificate, or
similar -- that a charm needs at runtime. Secrets have an owner (application,
unit, or model), one or more revisions (each holding content either inline or
in an external backend), and a permission record for each consumer that has
been granted access. Rotation policy governs when a new revision should be
created.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    SECRET -->|"has"| SECRET_METADATA["SECRET METADATA"]
    SECRET_METADATA -->|"has"| SECRET_REVISION["SECRET REVISION"]
    SECRET_REVISION -->|"stores content in"| SECRET_CONTENT["SECRET CONTENT (inline)"]
    SECRET_REVISION -->|"stores content via"| SECRET_VALUE_REF["SECRET VALUE REF (external backend)"]
    SECRET_METADATA -->|"has"| SECRET_ROTATION["SECRET ROTATION (schedule)"]

    APPLICATION -->|"owns"| SECRET
    UNIT -->|"owns"| SECRET
    MODEL -->|"owns"| SECRET

    UNIT -->|"consumes"| SECRET
    SECRET -->|"is granted to"| SECRET_PERMISSION["SECRET PERMISSION"]
    SECRET_PERMISSION -->|"grants access to"| APPLICATION
    SECRET_PERMISSION -->|"grants access to"| UNIT
```
*A secret has one or more revisions. Each revision stores its content either
inline in the model database or by reference in an external backend (Vault, a
Kubernetes secrets store). Consumers track which revision they have last seen
via the secret_unit_consumer record, which enables rotation notification.*

### Storage

Storage instances are the runtime records of storage attached to units. A charm
declares its storage needs in the charm storage spec; at deploy time those
declarations are resolved against a storage pool to produce storage instances.
A storage instance can back either a volume (block device) or a filesystem,
each with its own attachment record tracking the mount on the unit's machine.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    CHARM_STORAGE["CHARM STORAGE (spec)"] -->|"is declared by"| CHARM
    APPLICATION -->|"has directive"| APPLICATION_STORAGE_DIRECTIVE["APPLICATION STORAGE DIRECTIVE"]
    APPLICATION_STORAGE_DIRECTIVE -->|"references"| CHARM_STORAGE
    UNIT -->|"has directive"| UNIT_STORAGE_DIRECTIVE["UNIT STORAGE DIRECTIVE"]

    STORAGE_INSTANCE["STORAGE INSTANCE"] -->|"is provisioned from"| STORAGE_POOL["STORAGE POOL"]
    STORAGE_INSTANCE -->|"is owned by"| UNIT
    STORAGE_INSTANCE -->|"has lifecycle"| STORAGE_ATTACHMENT["STORAGE ATTACHMENT"]
    STORAGE_ATTACHMENT -->|"attaches to"| UNIT

    STORAGE_INSTANCE -->|"may be backed by"| STORAGE_VOLUME["STORAGE VOLUME"]
    STORAGE_INSTANCE -->|"may be backed by"| STORAGE_FILESYSTEM["STORAGE FILESYSTEM"]
    STORAGE_VOLUME -->|"is attached via"| STORAGE_VOLUME_ATTACHMENT["VOLUME ATTACHMENT"]
    STORAGE_FILESYSTEM -->|"is attached via"| STORAGE_FILESYSTEM_ATTACHMENT["FILESYSTEM ATTACHMENT"]
```
*A storage pool is the provisioning configuration (provider type and
parameters). A storage instance is what gets created from a pool when a unit
is deployed. It is realised as either a volume (block device) or a filesystem,
each with its own attachment record to the unit's machine.*

### Operations and actions

An operation is a user-initiated run of an action or an exec command against
one or more units or machines. Each operation has one or more tasks -- one per
targeted entity. An action operation references the charm action it invokes;
an exec operation does not.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    OPERATION -->|"may invoke"| CHARM_ACTION["CHARM ACTION"]
    OPERATION -->|"has"| OPERATION_TASK["OPERATION TASK"]
    OPERATION_TASK -->|"targets"| UNIT
    OPERATION_TASK -->|"targets"| MACHINE
    OPERATION_TASK -->|"produces"| OPERATION_TASK_OUTPUT["TASK OUTPUT"]
    OPERATION_TASK -->|"has"| OPERATION_TASK_STATUS["TASK STATUS"]
    OPERATION_TASK -->|"has"| OPERATION_TASK_LOG["TASK LOG"]
```
*An operation is what `juju run` creates. When targeted at multiple units,
one task record is created per unit. Tasks run in parallel by default;
the execution_group field on operation controls serial grouping.*

### Cross-model relations

A cross-model relation connects an application in one model (the consumer) to
an offer published by an application in another model (the offerer), possibly
on a different controller. On the offering side a remote consumer record is
created; on the consuming side a synthetic remote offerer application is
created. The offer connection record links the two sides through the offer.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    APPLICATION -->|"publishes"| OFFER
    OFFER -->|"is connected via"| OFFER_CONNECTION["OFFER CONNECTION"]
    OFFER_CONNECTION -->|"is backed by"| RELATION

    OFFER_CONNECTION -->|"has consumer side"| REMOTE_CONSUMER["APPLICATION REMOTE CONSUMER"]
    REMOTE_CONSUMER -->|"references consuming application in"| APPLICATION

    OFFER_CONNECTION -->|"has offerer side"| REMOTE_OFFERER["APPLICATION REMOTE OFFERER"]
    REMOTE_OFFERER -->|"is a synthetic application in"| APPLICATION

    EXTERNAL_CONTROLLER["EXTERNAL CONTROLLER"] -->|"is the controller of"| REMOTE_OFFERER
```
*The offer connection record is created when a consumer integrates with an
offer. On the offerer side an application_remote_consumer record tracks the
consuming application. On the consumer side a synthetic application_remote_offerer
record represents the remote application -- it has its own application record
so the rest of the model machinery treats it like any local application. The
external controller record provides the API address of the remote controller.*

### Leases and leadership

Leadership in Juju is implemented as a lease: the unit that holds the
application-leadership lease for an application is its leader. Leases are
held in the controller database, not the model database, so they can be
arbitrated across all models by a single authority. A lease has a holder,
a start time, and an expiry; agents renew leases before they expire.

```{mermaid}
%%{init: {"flowchart": {"htmlLabels": true}} }%%
flowchart TB
    LEASE -->|"is of type"| LEASE_TYPE["LEASE TYPE"]
    LEASE -->|"is held by"| HOLDER["HOLDER (unit name)"]
    LEASE -->|"is scoped to"| MODEL
    LEASE_TYPE -->|"is either"| APP_LEADERSHIP["application-leadership"]
    LEASE_TYPE -->|"or"| SINGULAR_CONTROLLER["singular-controller"]
    LEASE -->|"may be pinned by"| LEASE_PIN["LEASE PIN"]
```
*An application-leadership lease names the unit that is currently leader for
that application. A singular-controller lease ensures only one controller
worker handles a given task in an HA deployment. A lease pin prevents the
lease from expiring, used during upgrades and migrations to keep leadership
stable.*
