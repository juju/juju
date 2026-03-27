---
myst:
  html_meta:
    description: "Manage the Juju database: access controller and model databases, switch between them, run SQL queries, inspect schema using the REPL."
---

(manage-the-databases)=
# Manage the databases

You never need to interact with Juju's internal databases directly -- Juju handles creation, migrations, and replication automatically, and you normally query state through Juju commands (`juju status`, `juju config`, `juju show-unit`, etc.). However, direct database access is sometimes useful for debugging unexpected behavior, verifying relation data not surfaced through standard commands, or emergency recovery under guidance from Juju developers. This guide shows you how to access and query these databases through the REPL for inspection, debugging, and troubleshooting.

```{ibnote}
See more: {ref}`database`, {ref}`juju-db-repl`, [Dqlite documentation](https://dqlite.io/), [SQL documentation](https://www.sqlite.org/lang.html)
```

## Access the databases

Given a Juju 4.0+ client and controller, to access the databases:

1. SSH into a controller:

   ```text
   juju ssh -m controller 0
   ```

   In high-availability (HA) setups, you can SSH into any controller node.

2. Start the REPL:

   ```text
   juju_db_repl
   ```

   The REPL starts in the global `controller` database by default. You should see a prompt like:

   ```text
   repl (controller)>
   ```

3. To see the available REPL commands:

   ```text
   .help
   ```

   This shows a list of all available dot commands.

   ```{ibnote}
   See more: {ref}`juju-db-repl-help`
   ```

4. To exit:

   ```text
   .exit
   ```

   (You can also use `.quit`, or press `Ctrl+D`.)

   ```{ibnote}
   See more: {ref}`juju-db-repl-exit`
   ```

```{dropdown} Tip: Navigation
:color: success

Use the arrow keys to navigate through previous commands. Note: This is only possible per session -- the command history file is cleared when you exit the REPL.
```

## View the database cluster

To see the Dqlite cluster configuration:

```text
.describe-cluster
```

This shows the node IDs, addresses, and roles (leader/follower/spare) of the controllers nodes that form the Dqlite cluster. Note: With a single controller, this will show one node. In high-availability (HA) setups with multiple controllers, this will show all nodes and their current roles, which is useful for diagnosing cluster health and leadership status.

```{ibnote}
See more: {ref}`juju-db-repl-describe-cluster`
```

## View all the model databases

To view all the model databases:

```text
.models
```

Sample output:

```text
uuid                                  name
f5e8a7b2-1234-5678-90ab-cdef12345678  controller
a1b2c3d4-5678-90ab-cdef-123456789012  frontend
d87021ed-2121-4aa9-8d56-308f9bb0721c  backend
```

```{note}
The `.models` command only shows model databases, including the controller model database (shown as "controller" in the output above). The global `controller` database, which contains controller-level state, is not listed here. To access the global controller database, use `.switch controller`.
```

```{ibnote}
See more: {ref}`juju-db-repl-models`
```

## Switch to a database

The REPL starts in the global `controller` database. To switch to a different database:

- To access the global controller database:

  ```text
  .switch controller
  ```

- To access the controller model database:

  ```text
  .switch model-controller
  ```

- To access any other model database by name:

  ```text
  .switch model-frontend
  ```

- To access a model database by UUID (get UUIDs from `.models`):

  ```text
  .open f5e8a7b2-1234-5678-90ab-cdef12345678
  ```

The prompt updates to show the current database; e.g., `repl (model-frontend)>`

```{ibnote}
See more: {ref}`juju-db-repl-switch`, {ref}`juju-db-repl-open`
```

## View a database's schema

The REPL provides several commands to inspect database schema.

```{dropdown} Tip: Start with .tables
:color: success

Start with `.tables` to see what tables exist, then use other commands to drill into specific details.
```

To list all tables in the current database:

```text
.tables
```

Sample output from a model database:

```text
table_name
application
charm
machine
relation
unit
```

```{ibnote}
See more: {ref}`juju-db-repl-tables`
```

To see what columns and data types a table contains:

```text
.ddl application
```

This shows the CREATE statement that defines the table structure:

```sql
CREATE TABLE application (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life INTEGER NOT NULL,
    ...
)
```

```{ibnote}
See more: {ref}`juju-db-repl-ddl`
```

To list all views in the current database:

```text
.views
```

```{ibnote}
See more: {ref}`juju-db-repl-views`
```

To list all triggers in the current database:

```text
.triggers
```

```{ibnote}
See more: {ref}`juju-db-repl-triggers`
```

To see which tables reference a specific table and column:

```text
.fk-list unit uuid
```

To see reference counts for a specific UUID:

```text
.fk-list unit uuid abc-123-def-456
```

```{ibnote}
See more: {ref}`juju-db-repl-fk-list`
```

## Query a database

Once you've viewed a database's schema (using `.tables`, `.ddl`, etc.), you can query the data using standard SQL SELECT statements.

```{warning}
The REPL requires queries to be on a single line or use backslash `\` continuation for multi-line queries. For long queries, end each line (except the last) with a backslash.
```

```{ibnote}
See more: [SQL documentation](https://www.sqlite.org/lang.html)
```

## Query all the model databases

To run the same query on all model databases:

```sql
.query-models SELECT COUNT(*) FROM unit
```

The REPL executes the query on each model and displays results grouped by model UUID.

```{ibnote}
See more: {ref}`juju-db-repl-query-models`
```

## Modify a database

```{dropdown} About write operations
:color: warning

The REPL supports write operations (INSERT, UPDATE, DELETE), but these directly modify the database state and can corrupt your deployment. Only perform write operations when specifically directed by Juju developers or in emergency recovery scenarios. Always back up your controller first.
```

To modify data, use standard SQL write operations.

## View a database's change history

To view recent changes in the current database:

```text
.change-log
```

This shows the change log entries, useful for understanding recent modifications.

```{ibnote}
See more: {ref}`juju-db-repl-change-log`
```

````{dropdown} For Juju developers: View change stream
:color: info

To view change stream entries in the current database:

```text
.change-stream
```

This shows what the internal change stream will process.

```{ibnote}
See more: {ref}`juju-db-repl-change-stream`
```
````
