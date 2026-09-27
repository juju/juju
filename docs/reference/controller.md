---
myst:
  html_meta:
    description: "Juju controller reference: the controller as an entity -- declared by bootstrap, its singleton records in the controller database, its high-availability nodes, configuration, and watchers."
---

(controller)=
# Controller
```{audience} user
```

In Juju, the **controller** is the control plane: the running system that implements every change a {ref}`user <user>` asks for through a Juju client. It has a dual nature -- like the {ref}`client <client>` or a {ref}`unit agent <unit-agent>`, it is both an entity and running software:

- as an entity, the controller is a **singleton record** in the controller {ref}`database <database>` -- its identity, its configuration, and the records of its high-availability nodes;
- as software, it is the **{ref}`controller agent <controller-agent>`**: a `jujud` process whose workers run the Juju API server and an in-process embedded [Dqlite](https://canonical.com/dqlite) database. That half of the story -- the worker tree, high availability as a running system, upgrades -- is the controller agent's. This page carries the entity: how a controller is declared, what records it keeps, and what acts on it.

Its neighbours: the {ref}`controller model <the-controller-model>` that hosts it, the {ref}`models <model>` it serves as tenants, the {ref}`machines <machine>` that carry its controller nodes, and the {ref}`controller agent <controller-agent>` that is its running process.

## The controller in the declaration layer

The controller's declaration is the thinnest in this reference, and that is the finding: a controller is declared by {ref}`bootstrap <bootstrap-a-controller>` -- and it declares itself. The bootstrap client provisions a machine and installs `jujud` on it; the controller's own initialisation then writes the controller's records (the controller row is written by the controller-side bootstrap path -- the client never writes it directly), and it names itself from the bootstrap config: the client-chosen controller name arrives as the `controller-name` key of the {ref}`controller configuration <list-of-controller-configuration-keys>`, and is read back from there wherever the controller needs its name.

Access follows the same asymmetry: bootstrap itself has no controller access gate -- the controller does not exist yet, and the bootstrap client authenticates against the cloud instead. Every later verb on a controller is the same call over the controller API, gated controller-side: reading a controller (for example, `juju controllers`) requires {ref}`login access <user-access-controller-login>`; changing one -- its configuration, its destruction -- requires {ref}`superuser access <user-access-controller-superuser>`.

```{ibnote}
See more: {ref}`manage-controllers`
```

## The controller in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Controller attributes
:alt: The controller's stored tables as an entity-relationship slice: the singleton controller record at the centre -- uuid, the pointer to the controller model, target version, API port, TLS material; the model registry west with the namespace mapping under it; the HA node record east with its version satellite beside it and the API addresses below it. Each drawn line is a stored pointer; the controller row's only FK is the model pointer -- its nodes (and its configuration, not drawn: no FK to draw) are joined by the singleton convention.
:caption: Entity relationship diagram: The controller's stored records and the schema associations between them. The singleton row is enforced by the schema (a unique index over a constant -- exactly one controller). The model registry and the namespace mapping are the substrate recursion made concrete: the controller is itself a record inside a model database that its own database hosts. Nothing FK-points at the controller row except its model pointer: the HA nodes (and the configuration, prose-only -- no FK to draw) hang off the singleton convention; the node satellites (the per-node agent version, the API addresses) point at the node.
```

In the controller database the controller is a **singleton row** -- the schema enforces that exactly one exists. Rather than managing complex operational states on a single monolithic table, the controller's story distributes across a small set of records (`0004-controller.sql`, `0008-controller-config.sql`, `0010-controller-node.sql`):

- **The singleton row (`controller`)**: the controller's UUID; the pointer to the model it lives in (`model_uuid` -- the {ref}`controller model <the-controller-model>`, the one model that runs Juju itself); the target agent version an {ref}`upgrade <upgrading-things>` sets; the API port; and the TLS material (the certificate, its CA, the private keys) and the system identity.
- **The recursion**: the controller row is itself a record inside a model database that its own database hosts. The controller database keeps the **model registry** (`model`): one row per model, and for each a **namespace** (`model_namespace`) -- the name of the separate Dqlite database the model's records live in. The controller's own `model_uuid` points at the one model that is the controller model -- and in *that* model's database, the controller application is marked by `application_controller`, a singleton table under the same constant-index idiom as the controller row itself. Nothing is flagged: the controller model is *derived*, not stored -- `v_model` computes `is_controller_model` from the pointer, `v_cloud` derives `is_controller_cloud` from the qualified name (`controller`/`admin`), and the {ref}`controller machine <controller-machine>` is derived by the schema (`v_machine_is_controller`; see {ref}`the machine's records <the-machine-in-the-data-model>`).
- **The HA nodes (`controller_node`)**: one record per Dqlite node in the {ref}`high-availability <high-availability>` cluster -- the node's Dqlite identity and bind address, each uniquely indexed. The table has no pointer to the controller row: with exactly one controller, the membership hangs off the singleton convention, not a FK. The node's satellites point at the node: the agent **version** each node reports (an upgrade's progress, see {ref}`upgrading things <upgrading-things>`), the **API addresses** the nodes serve on (agent-facing and public scopes -- what agents use to re-orient as the cluster changes), and the node password.
- **The configuration (`controller_config`)**: a key/value satellite -- also without a pointer to the controller row, for the same reason. `v_controller_config` unions the stored keys with the values read back from the controller row (`controller-uuid`, `ca-cert`, `api-port`).
- **The record set**: the controller database is the controller's own record set: the controller row, its configuration, its nodes, and everything that lives controller-side rather than per-model -- {ref}`users <user>`, their {ref}`access levels <user-access-levels>`, {ref}`clouds <cloud>` and {ref}`credentials <credential>`, {ref}`SSH keys <ssh-key>`, {ref}`secret backends <secret-backend>`, leases, and the {ref}`migration <the-model-migration>` bookkeeping. The per-model databases are the other half (see {ref}`the full spine <data-model-full-spine>`).

The controller has two persistent stores: the Dqlite databases and blob storage. By default both are the filesystem of the controller's supporting infrastructure; either at bootstrap or later -- and, in a production setting, preferably from the start -- an S3-compatible object store can take over blob storage (e.g., AWS S3, MicroCeph, MinIO), through the object-store-related {ref}`controller configuration keys <controller-config-object-store-type>`.

A controller has no state machine and no subtypes: there is no life column and no status vocabulary on the controller row -- the controller is up, and its nodes' liveness is the Dqlite cluster's business (see {ref}`high availability <high-availability>`). High availability does not make a second controller: it makes more **controller nodes** running the same controller's database and API. One deployment, one controller.

(controller-persistence-rules)=
### Persistence rules and errors

The rules the stored records enforce:

- the controller database holds exactly one controller row -- the schema's singleton index enforces it;
- the controller model is derived, not marked: the pointer join (`v_model`'s `is_controller_model`) and the qualified name (`v_cloud`) are the derivations -- there is no controller-model flag to store;
- the controller's cloud cannot be removed while it still has {ref}`models <model>` (`cloud still in use` -- the deletion checks the registry and refuses while any model still points at the cloud).

(the-controllers-machinery)=
## The controller in the execution layer

By the time bootstrap returns, the controller is up -- the API server answering, the Dqlite cluster formed, the controller model and the `admin` {ref}`user <user>` created -- and every further verb on this page goes through the machinery it just started. The machinery itself is the {ref}`controller agent's <controller-agent>` story: its worker tree (the API server, the embedded Dqlite, the domain services, the provider tracker), high availability as a running system, and upgrades. This page carries only the operations on the entity and the surfaces it exposes.

(the-controller-operations)=
### Controller operations

(controller-bootstrap)=
#### Controller bootstrap

A controller comes into being through the {ref}`bootstrap <bootstrap-a-controller>` process: `juju bootstrap` turns an empty cloud into a running control plane. The mechanism and the state it leaves:

```{ggarch}
:file: ../juju.ggarch
:slides: Bootstrap machine | Bootstrap machine result
:caption: Bootstrapping a controller on a machine cloud: the mechanism and the state it leaves.
:slide-captions: Sequence diagram: The mechanism: the CLI authenticates against the cloud, provisions a virtual machine, installs jujud, and waits; the controller machine starts its controller agent, API server, and database, then reports the API ready. | Topology: The result: one controller, one model, no applications -- the controller machine running jujud, the API server and Dqlite in-process.
:alt: User invokes juju bootstrap. CLI authenticates with Cloud and provisions a VM. CLI installs jujud on the Controller machine. Controller machine starts the controller agent, API server, and database. Controller machine reports API ready. CLI reports Bootstrap complete to User. The resulting state is the controller machine alone: one controller, one model, no applications yet.
```

Bootstrap is also the controller record's creator: the initialised database gets the controller row, the `admin` {ref}`user <user>` and its superuser access, the controller model, and the bootstrapped {ref}`cloud <cloud>` and {ref}`credential <credential>` records.

#### Controller configuration

The controller configuration is the controller's key/value record, read and written through the controller config service -- and its changes are watched (see {ref}`controller watchers <the-controller-watchers>`). See {ref}`the list of controller configuration keys <list-of-controller-configuration-keys>` for the key list.

```{ibnote}
See more: {ref}`manage-controllers`
```

#### Controller nodes

The controller's nodes are the HA cluster's membership: adding a node brings another controller agent into the controller's Dqlite cluster (see {ref}`high availability <high-availability>`). There is no `juju enable-ha` client command -- the HighAvailability facade's `EnableHA` returns not-supported -- and the nodes are added and removed through the Terraform provider's `enable-ha` action.

(the-controller-watchers)=
### Controller watchers
```{audience} juju-dev
```

The controller side exposes these watch surfaces:

- **Controller configuration** -- the controller config record and the controller row; the controller's own workers reconcile on it.
- **Controller nodes** -- the HA cluster's membership.
- **The API addresses** -- the addresses the controller's nodes serve on, so agents can re-orient as the cluster changes.

Every watcher fires once immediately when it is created -- the initial query is the baseline snapshot -- and again on each qualifying change (see {ref}`the watcher pattern <watchers>`).
