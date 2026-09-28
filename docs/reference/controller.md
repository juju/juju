---
myst:
  html_meta:
    description: "Juju controller reference: the controller as software and entity -- declared by bootstrap, its singleton records in the controller database, its high-availability nodes, configuration, and watchers."
---

(controller)=
# Controller

In Juju, the **controller** is the control plane: the running system that implements every change a {ref}`user <user>` asks for. It is one object with a dual nature: the software boots first, and the entity is the state it materializes in the database as it runs its bootstrap lifecycle.

- **As software:** It is the **{ref}`controller agent <controller-agent>`**, a `jujud` process whose workers run the Juju API server and an embedded [Dqlite](https://canonical.com/dqlite) database. {ref}`Bootstrap <bootstrap-a-controller>` provisions a host machine, installs `jujud`, and starts it.
- **As an entity:** Through its bootstrap lifecycle, that software materializes its state as records in the controller {ref}`database <database>`: the **singleton controller record**, which carries its identity, its configuration, and the records of its high-availability nodes, and the **{ref}`application <application>`**, named `controller`, which it declares in the controller model it creates.

## The controller in the declaration layer

The declaration layer defines how a controller comes into being and how clients authenticate against it.

- **Bootstrap:** A controller is declared through the {ref}`bootstrap <bootstrap-a-controller>` process, which provisions a host machine, installs `jujud`, and initialises the controller's records. The bootstrap client authenticates directly against the target cloud; there is no controller to gate access yet. The controller declares itself: it names itself from the `controller-name` key of the {ref}`controller configuration <list-of-controller-configuration-keys>`, and it declares its own {ref}`application <application>`, named `controller`, in the controller model it creates.
- **Access gating:** Once running, every verb on a controller is a call over the controller API:
  - *Login access:* Reading controller state, for example listing controllers.
  - *Superuser access:* Changing the controller's configuration or destroying it.

```{ibnote}
See more: {ref}`manage-controllers`
```

## The controller in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Controller attributes
:alt: The singleton controller record at the centre with its salient columns; the model registry west with the namespace mapping under it; the HA node record east with its version satellite beside it and the API addresses below it. Each drawn line is a stored pointer; the controller record's only foreign key is the model pointer; nodes and configuration join by the singleton convention.
:caption: The controller's stored records. The controller is one record, a schema-enforced singleton, pointing at the controller model it lives in; the model registry names each model's database, and the HA nodes carry their version and API-address satellites; every line is a foreign key in one of those records.The controller's stored records. The controller is one record, a schema-enforced singleton, pointing at the controller model it lives in; the model registry names each model's database, and the HA nodes carry their version and API-address satellites; every line is a foreign key in one of those records.
```

In the controller database, the controller is a **singleton record**: the schema enforces that exactly one exists. The record set:

- **The controller has one record.** It carries the controller's UUID, the pointer to the internal controller model it lives in, the target agent version an {ref}`upgrade <upgrading-things>` sets, the API port, and the TLS material and system identity.
- **The controller has a model registry**: One record per model the controller serves, each naming the separate Dqlite database its records live in. The controller's own pointer names the controller model, the one model that runs Juju itself. Nothing is flagged: the controller model, the controller cloud, and the controller machine are derived by the schema's own views (see {ref}`the machine's records <the-machine-in-the-data-model>`).
- **The controller is also an {ref}`application <application>`.** In the controller model's database, the application named `controller` is deployed from the `juju-controller` {ref}`charm <charm>`; a sparse single-record marker marks it, schema-enforced to one entry by the same constant-index idiom as the controller record.
- **The controller charm's origin:** The charm archive (`controller.charm`) ships in the agent's data dir and bootstrap deploys it local-first, at revision 0 over a `local:` origin; if the archive is missing, bootstrap falls back to Charmhub.
- **The controller's units:** The application runs real units: one created at bootstrap, one per {ref}`HA node <high-availability>`. A machine is the controller machine when it hosts a unit of this application.
- **The controller has HA nodes**: One record per Dqlite node in the {ref}`high-availability <high-availability>` cluster: the node's Dqlite identity and bind address, each uniquely indexed. Each node's satellites record the agent version it reports, the API addresses it serves on for agents to re-orient as the cluster changes, and the node password.
- **The controller has configuration**: A key/value record set of controller-wide settings, read back through a derived view that unions the stored keys with values computed from the controller record.
- **Storage backends:** The controller's persistent data lives in the Dqlite databases and in blob storage. Both default to the controller's filesystem; an S3-compatible object store such as AWS S3, MicroCeph, or MinIO can take over blob storage through the object-store {ref}`controller configuration keys <controller-config-object-store-type>`.
- **The wider record set:** The controller database also holds everything that lives controller-side rather than per-model: {ref}`users <user>`, their {ref}`access levels <user-access-levels>`, {ref}`clouds <cloud>` and {ref}`credentials <credential>`, {ref}`SSH keys <ssh-key>`, {ref}`secret backends <secret-backend>`, leases, and the {ref}`migration <the-model-migration>` bookkeeping. The per-model databases are the other half (see {ref}`the full spine <data-model-full-spine>`).

The controller record has no life and no status vocabulary: the controller is up, and its nodes' liveness is the Dqlite cluster's business. High availability does not make a second controller; it makes more **controller nodes** running the same controller's database and API.

(the-controller-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - The database holds exactly one controller record; the schema enforces it with a unique index over a constant expression. HA expansion adds controller {ref}`nodes <high-availability>`, never a second controller.
  - The controller model and the controller cloud are derived, not marked: the schema's own views compute them from the records around the controller.
- **Errors:**
  - **`cloud still in use`:** Triggered when deleting a {ref}`cloud <cloud>` that one or more {ref}`models <model>` still reference. Remediation: move the models to another cloud or remove them before deleting.

(the-controllers-machinery)=
## The controller in the execution layer

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

  Bootstrap is also the controller record's creator: the initialised database gets the controller record, the `admin` {ref}`user <user>` and its superuser access, the controller model, and the bootstrapped {ref}`cloud <cloud>` and {ref}`credential <credential>` records.
- **Configuration:** The controller configuration is the controller's key/value record, read and written through the controller config service and watched for changes. See {ref}`the list of controller configuration keys <list-of-controller-configuration-keys>`.
- **HA expansion:** The nodes are the HA cluster's membership; adding a node brings another controller agent into the controller's Dqlite cluster. There is no `juju enable-ha` client command; the nodes are added and removed through the Terraform provider's `enable-ha` action.

```{ibnote}
See more: {ref}`manage-controllers`
```

(the-controller-watchers)=
### Controller watchers

The controller side exposes these watch surfaces:

- **Controller configuration:** The controller config record and the controller record; the controller's own workers reconcile on it.
- **Controller nodes:** The HA cluster's membership.
- **The API addresses:** The addresses the controller's nodes serve on, so agents can re-orient as the cluster changes.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
