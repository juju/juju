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

The **controller database** stores global controller-level information across all models:

- Controller configuration and metadata
- All model metadata (UUIDs, names, owners)
- User accounts and permissions
- Cloud and credential definitions
- High-availability cluster state

There is one controller database per controller. This is a separate global database, not associated with any specific model. It is accessed via the `controller` namespace in the {ref}`juju-db-repl`.

### Model databases

Each model (including the controller model) has its own **model database** containing that model's workload data:

- Applications and their configurations (including the controller application, in the case of the controller model)
- Units and their status
- Machines and their specifications
- Relations between applications
- Charm metadata and resources
- Secrets scoped to the model
- Storage and network spaces

Model databases are isolated -- changes in one model's database do not affect other models. They are accessed via the `model-<name>` namespace in the {ref}`juju-db-repl`.

## Database implementation

Starting with Juju 4.0, the database is implemented using [Dqlite](https://canonical.com/dqlite), an embedded, strongly-consistent distributed SQL database built on SQLite and the Raft consensus algorithm. Dqlite provides:

- **Embedded architecture**: Runs in-process within the controller with no separate database service.
- **Strong consistency**: Uses the Raft consensus algorithm to ensure consistent state across high-availability controller clusters.
- **SQL interface**: Supports standard SQL queries for inspection and debugging.
- **Transactional**: ACID-compliant transactions ensure data integrity.
- **Replicated**: Automatically replicates across controller nodes in HA deployments.