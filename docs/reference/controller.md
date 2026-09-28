---
myst:
  html_meta:
    description: "Juju controller reference: the controller as an entity -- declared by bootstrap, its singleton records in the controller database, its high-availability nodes, configuration, and watchers."
---

(controller)=
# Controller

In Juju, the **controller** is the control plane: the running system that implements every change a {ref}`user <user>` asks for. It has a dual nature:

- **As software:** It is the **{ref}`controller agent <controller-agent>`**, a `jujud` process whose workers run the Juju API server and an embedded [Dqlite](https://canonical.com/dqlite) database.
- **As an entity:** Through bootstrap, that software materializes as a **singleton record** in the controller {ref}`database <database>`: its identity, its configuration, and the records of its high-availability nodes. The worker tree, high availability as a running system, and upgrades are the controller agent's story; this page carries the entity.

## The controller in the declaration layer

```{audience} user, advanced-integrator
```

The declaration layer defines how a controller comes into being and how clients authenticate against it.

- **Bootstrap:** A controller is declared through the {ref}`bootstrap <bootstrap-a-controller>` process, which provisions a host machine, installs `jujud`, and initialises the controller's records. The bootstrap client authenticates directly against the target cloud; there is no controller to gate access yet. The controller declares itself: it names itself from the `controller-name` key of the {ref}`controller configuration <list-of-controller-configuration-keys>`.
- **Access gating:** Once running, every verb on a controller is a call over the controller API:
  - *Login access:* Reading controller state, for example listing controllers.
  - *Superuser access:* Changing the controller's configuration or destroying it.

```{ibnote}
See more: {ref}`manage-controllers`
```

## The controller in the persistence layer

```{audience} charm-dev, juju-dev, advanced-integrator
```

```{ggarch}
:file: ../juju.ggarch
:view: Controller attributes
:alt: The controller's stored tables as an entity-relationship slice: the singleton controller record at the centre -- uuid, the pointer to the controller model, target version, API port, TLS material; the model registry west with the namespace mapping under it; the HA node record east with its version satellite beside it and the API addresses below it. Each drawn line is a stored pointer; the controller row's only FK is the model pointer; nodes and configuration join by the singleton convention.
:caption: Entity relationship diagram: The controller entity schema and relational associations. Notation: each line starts at the foreign key holding the pointer; 1/m cardinality at each end. Core tables: `controller` anchors the entity as a schema-enforced singleton, linked to `model` (the registry; the controller model pointer is the row's only FK) with `model_namespace` under it, and `controller_node` for the HA cluster with its per-node satellites. Omitted for clarity: the configuration key/value rows (`controller_config`, no FK to draw).
```

In the controller database, the controller is a **singleton row**: the schema enforces that exactly one exists. The record set (`0004-controller.sql`, `0008-controller-config.sql`, `0010-controller-node.sql`):

- **The singleton row (`controller`)**: The controller's UUID, the pointer to the internal controller model it lives in, the target agent version an {ref}`upgrade <upgrading-things>` sets, the API port, and the TLS material and system identity.
- **The model registry (`model`, `model_namespace`)**: One row per model the controller serves, each naming the separate Dqlite database its records live in. The controller's own `model_uuid` points at the controller model, the one model that runs Juju itself; in that model's database, `application_controller` marks the controller application. Nothing is flagged: the controller model, the controller cloud, and the controller machine are derived by schema views (`v_model`, `v_cloud`, `v_machine_is_controller`; see {ref}`the machine's records <the-machine-in-the-data-model>`).
- **The HA nodes (`controller_node`)**: One record per Dqlite node in the {ref}`high-availability <high-availability>` cluster: the node's Dqlite identity and bind address, each uniquely indexed. Each node's satellites record the agent version it reports, the API addresses it serves on for agents to re-orient as the cluster changes, and the node password.
- **The configuration (`controller_config`)**: A key/value satellite of controller-wide settings. `v_controller_config` unions the stored keys with values read from the controller row: `controller-uuid`, `ca-cert`, `api-port`.
- **Storage backends:** The controller's persistent data lives in the Dqlite databases and in blob storage. Both default to the controller's filesystem; an S3-compatible object store such as AWS S3, MicroCeph, or MinIO can take over blob storage through the object-store {ref}`controller configuration keys <controller-config-object-store-type>`.
- **The record set:** The controller database also holds everything that lives controller-side rather than per-model: {ref}`users <user>`, their {ref}`access levels <user-access-levels>`, {ref}`clouds <cloud>` and {ref}`credentials <credential>`, {ref}`SSH keys <ssh-key>`, {ref}`secret backends <secret-backend>`, leases, and the {ref}`migration <the-model-migration>` bookkeeping. The per-model databases are the other half (see {ref}`the full spine <data-model-full-spine>`).

The controller row has no life column and no status vocabulary: the controller is up, and its nodes' liveness is the Dqlite cluster's business. High availability does not make a second controller; it makes more **controller nodes** running the same controller's database and API.

(the-controller-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - The database holds exactly one controller row; the schema enforces it with the `idx_singleton_controller` unique index over a constant expression. HA expansion adds controller {ref}`nodes <high-availability>`, never a second controller.
  - The controller model and the controller cloud are derived, not marked: `v_model` computes `is_controller_model` from the `model_uuid` join, `v_cloud` computes `is_controller_cloud` from the controller model's cloud.
- **Errors:**
  - **`cloud still in use`:** Triggered when deleting a {ref}`cloud <cloud>` that one or more {ref}`models <model>` still reference. Remediation: move the models to another cloud or remove them before deleting.

(the-controllers-machinery)=
## The controller in the execution layer

```{audience} user, advanced-integrator, charm-dev, juju-dev
```

By the time bootstrap returns, the controller is up: the API server answering, the Dqlite cluster formed, the controller model and the `admin` {ref}`user <user>` created. The machinery is the {ref}`controller agent's <controller-agent>` story. This page carries the operations on the entity and the surfaces it exposes.

(the-controller-operations)=
### Controller operations

- **Bootstrap:** `juju bootstrap` turns an empty cloud into a running control plane. The mechanism and the state it leaves, in the two diagrams:

```{ggarch}
:file: ../juju.ggarch
:slides: Bootstrap machine | Bootstrap machine result
:caption: Bootstrapping a controller on a machine cloud: the mechanism and the state it leaves.
:slide-captions: Sequence diagram: The mechanism: the CLI authenticates against the cloud, provisions a virtual machine, installs jujud, and waits; the controller machine starts its controller agent, API server, and database, then reports the API ready. | Topology: The result: one controller, one model, no applications -- the controller machine running jujud, the API server and Dqlite in-process.
:alt: User invokes juju bootstrap. CLI authenticates with Cloud and provisions a VM. CLI installs jujud on the Controller machine. Controller machine starts the controller agent, API server, and database. Controller machine reports API ready. CLI reports Bootstrap complete to User. The resulting state is the controller machine alone: one controller, one model, no applications yet.
```

  Bootstrap is also the controller record's creator: the initialised database gets the controller row, the `admin` {ref}`user <user>` and its superuser access, the controller model, and the bootstrapped {ref}`cloud <cloud>` and {ref}`credential <credential>` records.
- **Configuration:** The controller configuration is the controller's key/value record, read and written through the controller config service and watched for changes. See {ref}`the list of controller configuration keys <list-of-controller-configuration-keys>`.
- **HA expansion:** The nodes are the HA cluster's membership; adding a node brings another controller agent into the controller's Dqlite cluster. There is no `juju enable-ha` client command; the nodes are added and removed through the Terraform provider's `enable-ha` action.

```{ibnote}
See more: {ref}`manage-controllers`
```

(the-controller-watchers)=
### Controller watchers

The controller side exposes these watch surfaces:

- **Controller configuration:** The controller config record and the controller row; the controller's own workers reconcile on it.
- **Controller nodes:** The HA cluster's membership.
- **The API addresses:** The addresses the controller's nodes serve on, so agents can re-orient as the cluster changes.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
