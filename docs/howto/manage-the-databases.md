---
myst:
  html_meta:
    description: "Manage the Juju databases: access controller and model databases, switch between them, run SQL queries, inspect schema using the Dqlite REPL."
---

(manage-the-databases)=
# Manage the databases

Juju stores its state in embedded Dqlite databases -- one for the controller model and one for each additional model. The controller model database stores controller-level data (users, clouds, model metadata), while each model database stores that model's applications, units, machines, and relations.

You can access and query these databases through a Dqlite REPL, which provides direct SQL access for inspection, debugging, and troubleshooting.

```{ibnote}
See more: {ref}`database`, {ref}`juju_db_repl`, [Dqlite documentation](https://dqlite.io/), [SQL documentation](https://www.sqlite.org/lang.html)
```

This guide shows you how to work with databases (requires SSH access to a Juju 4.0+ controller).

## Access databases

To work with databases, access the Dqlite REPL on any controller machine.

1. SSH into a controller machine:

   ```text
   juju ssh -m controller 0
   ```

   In high-availability (HA) setups, you can SSH into any controller node.

2. Start the REPL:

   ```text
   juju_db_repl
   ```

   ```{dropdown} Troubleshooting: juju_db_repl command not found

   The `juju_db_repl` helper function is defined in `/etc/profile.d/juju-introspection.sh`. If it's not available:

   1. **Source the file manually:**
      \`\`\`text
      source /etc/profile.d/juju-introspection.sh
      juju_db_repl
      \`\`\`

   2. **Or start a login shell (then run `juju_db_repl` inside it):**
      \`\`\`text
      bash -l
      # Now you're in a new shell, run:
      juju_db_repl
      \`\`\`

   3. **Or call `jujud` directly (for machine 0):**
      \`\`\`text
      sudo /var/lib/juju/tools/machine-0/jujud db-repl --machine-id=0
      \`\`\`

      For other machine IDs, replace `machine-0` and `--machine-id=0` accordingly.
   ```

   The REPL starts in the controller model database by default. You should see a prompt like:

   ```text
   repl (controller)>
   ```

3. To see available REPL commands:

   ```text
   .help
   ```

   (You can also use `.h` as a shortcut.)

   ```{ibnote}
   See more: {ref}`juju_db_repl-help`
   ```

4. To exit:

   ```text
   .exit
   ```

   (You can also use `.quit`, or press `Ctrl+D` or `Ctrl+C`.)

   ```{ibnote}
   See more: {ref}`juju_db_repl-exit`
   ```

```{dropdown} Tip: Navigation
Use arrow keys to navigate command history. The REPL maintains a history file for session continuity.
```

## View the database cluster

To see the Dqlite cluster configuration:

```text
.describe-cluster
```

```{ibnote}
See more: {ref}`juju_db_repl-describe-cluster`
```

This shows the node IDs, addresses, and roles (leader/follower/spare) of the controller machines that form the Dqlite database cluster. With a single controller, this shows one node. In high-availability (HA) setups with multiple controllers, this shows all nodes and their current roles, which is useful for diagnosing cluster health and leadership status.

## List databases

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
Model databases in the REPL are prefixed with `model-`. For a model named `production`, use `.switch model-production`. For a model named `model-1`, use `.switch model-model-1`.
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

To switch back to the controller model database:

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

```{dropdown} Reminder: What's a view?
A view is a virtual table defined by a SQL query. Unlike regular tables, views don't store data themselves -- they present data from other tables in a specific way.
```

```text
.views
```

To list all triggers in the current database:

```{dropdown} Reminder: What's a trigger?
A trigger is a database object that automatically executes code when certain events occur (INSERT, UPDATE, DELETE). Juju uses triggers internally to maintain data consistency and track changes.
```

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

```{dropdown} Tip: Viewing query results
After running a query, press `Tab` to cycle through different result formatting options. Use a backslash (`\`) at the end of a line to continue multi-line queries.
```

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

### Query a model database

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

To run a multi-line query, use a backslash at the end of each line to continue:

```text
SELECT a.name, COUNT(u.uuid) as num_units \
FROM application a \
LEFT JOIN unit u ON a.uuid = u.application_uuid \
GROUP BY a.name;
```

## Query across databases

To run the same query on all model databases:

```text
.query-models SELECT COUNT(*) FROM unit
```

The REPL executes the query on each model and displays results grouped by model UUID.

## Modify a database

```{dropdown} About write operations
:color: warning

The REPL supports write operations (INSERT, UPDATE, DELETE), but these directly modify the database state and can corrupt your deployment. Only perform write operations when specifically directed by Juju developers or in emergency recovery scenarios. Always back up your controller first using `juju create-backup`.
```

To modify data, use standard SQL write operations:

```text
INSERT INTO table_name (column1, column2) VALUES (value1, value2);
UPDATE table_name SET column1 = value1 WHERE condition;
DELETE FROM table_name WHERE condition;
```

## View a database's change history

To view recent changes in the current database:

```text
.change-log
```

This shows the change log entries, useful for understanding recent modifications.

To view change stream entries in the current database:

```text
.change-stream
```

This shows what the internal change stream will process.
