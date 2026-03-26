---
myst:
  html_meta:
    description: "Manage the Juju database: access controller and model databases, switch between them, run SQL queries, inspect schema using the REPL."
---

(manage-the-databases)=
# Manage the databases

Juju stores its state in Dqlite. This is segmented into different databases: There is a global controller database and then a model database for each model, where
- the controller model's database stores controller-level data (users, clouds, model metadata)
- each workload model's database stores that model's applications, units, machines, and relations.

This guide shows you how to access and query these databases through a REPL, which provides direct SQL access for inspection, debugging, and troubleshooting.

```{ibnote}
See more: {ref}`database`, {ref}`juju_db_repl`, [Dqlite documentation](https://dqlite.io/), [SQL documentation](https://www.sqlite.org/lang.html)
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

   The REPL starts in the controller database by default. You should see a prompt like:

   ```text
   repl (controller)>
   ```

3. To see the available REPL commands:

   ```text
   .help
   ```

   ```{ibnote}
   See more: {ref}`juju_db_repl-help`
   ```

4. To exit:

   ```text
   .exit
   ```

   (You can also use `.quit`, or press `Ctrl+D`.)

   ```{ibnote}
   See more: {ref}`juju_db_repl-exit`
   ```

```{dropdown} Tip: Navigation
Use the arrow keys to navigate through previous commands. Note: This is only possible per session -- the command history file is cleared when you exit the REPL.
```

## View the database cluster

To see the Dqlite cluster configuration:

```text
.describe-cluster
```

```{ibnote}
See more: {ref}`juju_db_repl-describe-cluster`
```

This shows the node IDs, addresses, and roles (leader/follower/spare) of the controllers nodes that form the Dqlite cluster. Note: With a single controller, this will show one node. In high-availability (HA) setups with multiple controllers, this will show all nodes and their current roles, which is useful for diagnosing cluster health and leadership status.

## List the databases

To see all available databases (controller model and other models):

```text
.models
```

```{ibnote}
See more: {ref}`juju_db_repl-models`
```

Sample output:

```text
uuid                                  name
f5e8a7b2-1234-5678-90ab-cdef12345678  controller
a1b2c3d4-5678-90ab-cdef-123456789012  default
d87021ed-2121-4aa9-8d56-308f9bb0721c  model-1
```

## Switch to a database

```{note}
Model databases in the REPL are prefixed with `model-`. For a model named `production`, use `.switch model-production`.
```

To switch to a model database by its name:

```text
.switch model-default
```

```{ibnote}
See more: {ref}`juju_db_repl-switch`
```

To switch to a model database by its UUID (useful when you don't know the model name):

```text
.open f5e8a7b2-1234-5678-90ab-cdef12345678
```

```{ibnote}
See more: {ref}`juju_db_repl-open`
```

To switch back to the controller database:

```text
.switch controller
```

The prompt updates to show the current database:

```text
repl (model-default)>
```

## View a database's schema

To list all tables in the current database:

```text
.tables
```

```{ibnote}
See more: {ref}`juju_db_repl-tables`
```

```{dropdown} Tip: Start with .tables
Run `.tables` when you first switch to a database to understand its schema before writing queries.
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

To see what columns and data types a table contains:

```text
.ddl application
```

This shows the CREATE statement that defines the table structure:

```text
CREATE TABLE application (
    uuid TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    life INTEGER NOT NULL,
    ...
)
```

To list all views in the current database:

```text
.views
```

To list all triggers in the current database:

```text
.triggers
```

To see which tables reference a specific table and column:

```text
.fk-list unit uuid
```

To see reference counts for a specific UUID:

```text
.fk-list unit uuid abc-123-def-456
```

## Query a database

### Query the controller model database

To find models on a specific cloud:

```text
SELECT uuid, name, cloud_name, cloud_region
FROM model
WHERE cloud_name = 'aws';
```

To list all users:

```text
SELECT name, display_name, created_at
FROM user;
```

### Query a workload model database

To view all applications:

```text
SELECT name, life FROM application;
```

To count units by application:

```text
SELECT application_uuid, COUNT(*) as unit_count
FROM unit
GROUP BY application_uuid;
```

To view applications and their charm URLs:

```text
SELECT a.name, c.reference_name
FROM application a
JOIN charm c ON a.charm_uuid = c.uuid;
```

To check machine status:

```text
SELECT machine_id, life, instance_id
FROM machine;
```

To view relations between applications:

```text
SELECT name, life
FROM relation;
```

For multi-line queries, use standard SQL line continuation with backslashes.

## Query across the databases

To run the same query on all model databases:

```text
.query-models SELECT COUNT(*) FROM unit
```

The REPL executes the query on each model and displays results grouped by model UUID.

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

```{dropdown} For Juju developers: View change stream

To view change stream entries in the current database:

\`\`\`text
.change-stream
\`\`\`

This shows what the internal change stream will process.
```
