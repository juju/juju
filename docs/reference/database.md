---
myst:
  html_meta:
    description: "Juju database explanation: Dqlite-based state management architecture with controller and model databases for multi-cloud orchestration."
---

(database)=
# Database

```{ibnote}
See also: {ref}`manage-the-databases`
```

In Juju, the **database** is the persistent storage layer that maintains all state information about controllers, models, applications, units, relations, and other entities in a Juju deployment. It is the source of truth for the current state of your infrastructure.

## Database architecture

The Juju controller organizes data across multiple isolated databases:

### Controller database

The **controller database** stores global controller-level information across all models, including but not limited to:

- Controller configuration and metadata
- All model metadata (UUIDs, names, owners)
- User accounts and permissions
- Cloud and credential definitions
- High-availability cluster state

There is one controller database per controller. This is a separate global database, not associated with any specific model. It is accessed via the `controller` namespace in the {ref}`juju-db-repl`.

### Model databases

Each model (including the controller model) has its own **model database** containing that model's workload data, including but not limited to:

- Applications and their configurations (including the controller application, in the case of the controller model)
- Units and their status
- Machines and their specifications
- Relations between applications
- Charm metadata and resources
- Secrets
- Storage and network spaces

Model databases are isolated -- changes in one model's database do not affect other models. They are accessed via the `model-<name>` namespace in the {ref}`juju-db-repl`.

```{ggarch}
:file: ../juju.ggarch
:view: Data model (full spine)
:alt: Record nodes for user, cloud, credential, model, application, charm, unit, machine and relation, each with its primary key and foreign-key fields, and arrows showing every pointer: user owns clouds and credentials, model contains applications and uses a credential, application references its charm and has units, unit runs on a machine, relation connects applications.
:caption: What a model database holds. Every box is a Dqlite table in the model database; every arrow is a foreign key stored in the table it leaves — the pointer location the storage layer actually has. The same walk, grounded field by field in the model's DDL, is what the {ref}`juju-db-repl` reads.
```

## Database implementation

Starting with Juju 4.0, the database is implemented using [Dqlite](https://canonical.com/dqlite), an embedded, strongly-consistent distributed SQL database built on SQLite and the Raft consensus algorithm. Dqlite provides:

- **Embedded architecture**: Runs in-process within the controller with no separate database service.
- **Strong consistency**: Uses the Raft consensus algorithm to ensure consistent state across high-availability controller clusters.
- **SQL interface**: Supports standard SQL queries for inspection and debugging.
- **Transactional**: ACID-compliant transactions ensure data integrity.
- **Replicated**: Automatically replicates across controller nodes in HA deployments.

```{ggarch}
:file: ../juju.ggarch
:view: HA controller: Dqlite replicaset
:alt: Three machine nodes side by side, each running a controller agent with an embedded Dqlite database, connected by replicate-arrows between the databases.
:caption: The controller database under high availability: three machines, each running a controller agent with Dqlite embedded in-process. There is no separate database service — the controllers raft-replicate one database among themselves, which is why the guarantees above hold.
```
