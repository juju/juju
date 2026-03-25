---
myst:
  html_meta:
    description: "Juju database explanation: Dqlite-based state management architecture with controller and model databases for multi-cloud orchestration."
---

(database)=
# Database

In Juju, the **database** is the persistent storage layer that maintains all state information about controllers, models, applications, units, relations, and other entities in a Juju deployment. It is the source of truth for the current state of your infrastructure.

```{versionadded} 4.0
Starting with Juju 4.0, the database backend changed from MongoDB to [Dqlite](https://canonical.com/dqlite), an embedded, strongly-consistent distributed SQL database built on SQLite and the Raft consensus algorithm.
```

## Why Dqlite?

The move from MongoDB to Dqlite in Juju 4.0 brought several advantages:

- **Embedded architecture** -- Dqlite runs in-process within the controller, eliminating the need for a separate database service
- **Strong consistency** -- Built on the Raft consensus algorithm, ensuring consistent state across high-availability controller clusters
- **Reduced operational complexity** -- No separate database service to manage, monitor, or secure
- **Lower resource footprint** -- More efficient memory and storage usage compared to MongoDB
- **SQL interface** -- Standard SQL queries for inspection and debugging

## Database architecture

The Juju controller organizes data across multiple isolated databases:

### Controller database

The **controller database** (also called the "controller model database") stores controller-level information:

- Controller configuration and metadata
- All model metadata (UUIDs, names, owners)
- User accounts and permissions
- Cloud and credential definitions
- High-availability cluster state

There is one controller database per controller, accessed via the `controller` namespace in the {ref}`juju_db_repl`.

### Model databases

Each model has its own **model database** containing model-specific data:

- Applications and their configurations
- Units and their status
- Machines and their specifications
- Relations between applications
- Charm metadata and resources
- Secrets scoped to the model
- Storage and network spaces

Model databases are isolated -- changes in one model's database do not affect other models. They are accessed via the `model-<uuid>` namespace in the {ref}`juju_db_repl`.

```{note}
This multi-database architecture provides strong isolation between models and allows Juju to scale to many models on a single controller without performance degradation.
```

## The database and Juju's state management

The database is central to how Juju manages state:

1. **Agents read state** -- Controller agents, machine agents, and unit agents query the database to understand the desired state of the system
2. **Agents write state** -- Agents update the database with observed state (status, IP addresses, etc.)
3. **State reconciliation** -- Juju continuously compares desired state (what you've commanded) with observed state (what agents report) and takes action to reconcile differences
4. **Relation data** -- When applications relate, they exchange data through dedicated relation databags stored in the database
5. **Stored state** -- Charms can persist data between runs using stored state, which is saved in the database

```{dropdown} Reminder: State persistence for charms
:color: info

The database is the only reliable persistence layer for charm state. When a charm executes in response to a hook, it has no persistent filesystem between runs. Any data the charm needs to remember must be stored either:

- In relation data (shared with other charms via the database)
- In stored state (private to the charm, also stored in the database)
- In an external service the charm integrates with

This is why the controller database is so critical -- it's the memory of your Juju deployment.
```

## When to access the database directly

Under normal circumstances, you interact with the database indirectly through Juju commands (`juju status`, `juju config`, `juju show-unit`, etc.). The Juju CLI, agents, and API server all read from and write to the database on your behalf.

However, there are situations where you might need direct database access:

**Debugging and inspection:**
- Investigating unexpected behavior by examining raw state
- Verifying relation data that isn't surfaced through standard commands
- Understanding internal Juju state for development or troubleshooting

**Operational insight:**
- Querying historical change logs to track modifications
- Inspecting database schema to understand Juju's data model
- Gathering information across multiple models efficiently

**Emergency recovery:**
- Correcting database inconsistencies under guidance from Juju developers
- Recovering from specific failure scenarios where normal operations are blocked

```{warning}
Direct database modification (INSERT, UPDATE, DELETE operations) can corrupt your controller's state and should only be performed when directed by Juju developers or in documented emergency recovery scenarios. Always back up your controller before performing write operations.
```

## Database access and security

The Dqlite database is protected by several security measures:

- **Authentication required** -- Only authenticated controller agents and administrators with SSH access to controller machines can access the database
- **No network exposure** -- The database does not listen on network ports accessible outside the controller
- **Encrypted passwords** -- All passwords stored in the database are hashed and salted
- **High-availability replication** -- In HA deployments, the database is replicated across controller nodes using encrypted Raft communication

To access the database for inspection and debugging, you use the {ref}`juju_db_repl` tool, an interactive SQL shell available on controller machines.

```{ibnote}
See more: {ref}`juju_db_repl`, {ref}`manage-the-databases`
```

## Database operations and management

Juju handles most database operations automatically:

- **Creation** -- The controller database is created during `juju bootstrap`; model databases are created with `juju add-model`
- **Schema migrations** -- Juju automatically applies schema changes when you upgrade the controller
- **Replication** -- In high-availability setups, Dqlite automatically replicates data across controller nodes
- **Cleanup** -- Model databases are marked for cleanup when you destroy a model (though physical deletion is not yet implemented)

For operational tasks like querying the database or viewing cluster status, see the how-to guide for managing databases.

```{ibnote}
See more: {ref}`manage-the-databases`
```
