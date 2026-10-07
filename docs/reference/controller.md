---
myst:
  html_meta:
    description: "Juju controller reference: the controller as software and entity -- declared by bootstrap, its singleton records in the controller database, its high-availability nodes, configuration, and watchers."
---

(controller)=
# Controller

In Juju, the **controller** is the control plane: the running system that implements every change a {ref}`user <user>` asks for. It is one object with a dual nature: the software boots first, and the entity is the state it materializes in the database as it runs its bootstrap lifecycle.

- **As software:** It is the **{ref}`controller agent <controller-agent>`**, a `jujuagentd` process whose workers run the Juju API server and an embedded [Dqlite](https://canonical.com/dqlite) database. {ref}`Bootstrap <bootstrap-a-controller>` provisions a host machine, installs `jujuagentd`, and starts it.
- **As an entity:** Through its bootstrap lifecycle, that software materializes its state as records in the controller {ref}`database <database>`: the **singleton controller record**, which carries its identity, its configuration, and the records of its high-availability nodes, and the **{ref}`application <application>`**, named `controller`, which it declares in the controller model it creates.

## Controller in the declaration layer

The declaration layer defines how a controller comes into being and how clients authenticate against it.

- **Bootstrap:** A controller is declared through the {ref}`bootstrap <bootstrap-a-controller>` process, which provisions a host machine, installs `jujuagentd`, and initialises the controller's records. The bootstrap client authenticates directly against the target cloud; there is no controller to gate access yet. The controller declares itself: it names itself from the `controller-name` key of the {ref}`controller configuration <list-of-controller-configuration-keys>`, and it declares its own {ref}`application <application>`, named `controller`, in the controller model it creates.
- **Access gating:** Once running, every verb on a controller is a call over the controller API.
  - *Login access:* Reading controller state, for example listing controllers.
  - *Superuser access:* Changing the controller's configuration or destroying it.
- **Removal:** Destroying a controller needs superuser access. The cloud the controller was bootstrapped on cannot be removed while the controller stands (see {ref}`cloud <cloud>`).

```{ibnote}
See more: {ref}`manage-controllers`
```

(the-controller-persistence-rules)=
## Controller in the persistence layer

In the {ref}`controller database <database>`, the controller is a **singleton record**: the schema enforces that exactly one exists. The record set:

- **Controller record:** It carries the controller's UUID, the pointer to the internal controller model it lives in, the target agent version an {ref}`upgrade <upgrade-your-deployment>` sets, the API port, and the TLS material and system identity.
  - *Rule:* The database holds exactly one controller record; the schema enforces it with a unique index over a constant expression. HA expansion adds controller {ref}`nodes <high-availability>`, never a second controller.
  - *Rule:* The controller has no life and no status vocabulary: the controller is up, and its nodes' liveness is the Dqlite cluster's business.
- **Model registry:** One record per model the controller serves, each naming the separate Dqlite database its records live in. The controller's own pointer names the controller model, the one model that runs Juju itself.
  - *Rule:* Nothing is flagged: the controller model, the controller cloud, and the controller machine are derived by the schema's own views (see {ref}`the machine's records <the-machine-in-the-data-model>`).
- **The `controller` application:** The controller is also an {ref}`application <application>`. In the controller model's database, the application named `controller` is deployed from the `juju-controller` {ref}`charm <charm>`; a sparse single-record marker marks it, schema-enforced to one entry by the same constant-index idiom as the controller record.
  - *Charm origin:* The charm archive (`controller.charm`) ships in the agent's data dir and bootstrap deploys it local-first, at revision 0 over a `local:` origin; if the archive is missing, bootstrap falls back to Charmhub.
  - *Units:* The application runs real units: one created at bootstrap, one per {ref}`HA node <high-availability>`. A machine is the controller machine when it hosts a unit of this application.
- **HA nodes:** One record per Dqlite node in the {ref}`high-availability <high-availability>` cluster: the node's Dqlite identity and bind address, each uniquely indexed. Each node's satellites record the agent version it reports, the API addresses it serves on for agents to re-orient as the cluster changes, and the node password. High availability does not make a second controller; it makes more **controller nodes** running the same controller's database and API.
- **Configuration:** A key/value record set of controller-wide settings, read back through a derived view that unions the stored keys with values computed from the controller record.
- **Storage backends:** The controller's persistent data lives in the Dqlite databases and in blob storage. Both default to the controller's filesystem (the `file` object-store backend); an S3-compatible object store such as AWS S3, MicroCeph, or MinIO can take over blob storage: the controller database keeps the object-store backend records, with the backend type and, for S3, its endpoint, credentials and region.
- **The wider record set:** The controller database also holds everything that lives controller-side rather than per-model: {ref}`users <user>`, their {ref}`access levels <user-access-levels>`, {ref}`clouds <cloud>` and {ref}`credentials <credential>`, {ref}`SSH keys <ssh-key>`, {ref}`secret backends <secret-backend>`, leases, and the {ref}`migration <the-model-migration>` bookkeeping. The per-model databases are the other half (see {ref}`the database <database>`).

(the-controllers-machinery)=
## Controller in the execution layer

By the time bootstrap returns, the controller is up: the API server answering, the Dqlite cluster formed, the controller model and the `admin` {ref}`user <user>` created. The machinery is the {ref}`controller agent's <controller-agent>` story. This page carries the operations on the entity and the surfaces it exposes.

(the-controller-operations)=
### Controller operations

- **Bootstrap:** `juju bootstrap` turns an empty cloud into a running control plane. The CLI authenticates against the cloud, provisions a virtual machine, installs `jujuagentd` and waits; the controller machine starts its controller agent, API server and database, then reports the API ready. The result is one controller, one model and no applications: the controller machine running `jujuagentd`, with the API server and Dqlite in-process. Bootstrap is also the controller record's creator: the initialised database gets the controller record, the `admin` {ref}`user <user>` and its superuser access, the controller model, and the bootstrapped {ref}`cloud <cloud>` and {ref}`credential <credential>` records.
- **Configuration:** The controller configuration is the controller's key/value record, read and written through the controller config service and watched for changes. See {ref}`the list of controller configuration keys <list-of-controller-configuration-keys>`.
- **HA expansion:** The nodes are the HA cluster's membership; adding a node brings another controller agent into the controller's Dqlite cluster. Juju 4.1 has no `juju enable-ha` command; the `EnableHA` call on the controller API adds and removes the nodes.

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
