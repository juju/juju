---
myst:
  html_meta:
    description: "juju_db_repl reference: Dqlite REPL interactive SQL shell for querying Juju controller and model databases."
---

(juju_db_repl)=
# `juju_db_repl`


```{versionadded} 4.0
```

```{ibnote}
See also: {ref}`database`, {ref}`manage-the-databases`
```

`juju_db_repl` is the Dqlite REPL (Read-Eval-Print Loop), an interactive SQL shell for querying Juju's embedded [Dqlite](https://canonical.com/dqlite) databases. Starting with Juju 4.0, the controller uses Dqlite as its embedded database system instead of MongoDB. The REPL provides direct access to the controller's relational database for inspection, debugging, and administrative tasks.

Access it on any controller machine via SSH using the `juju_db_repl` helper command. In high-availability (HA) deployments, you can SSH into any controller node to access it.

The REPL provides an interactive interface to query the database using standard SQL syntax (SELECT, INSERT, UPDATE, DELETE) along with special dot commands for navigating between databases and introspecting the schema. Query features include multi-line queries (use `\` to continue), command history navigation (use arrow keys to browse previous commands), and formatted table output. It is a customized version built on top of the upstream [Dqlite](https://dqlite.io/) project, adding Juju-specific features like model-aware database switching, schema introspection for Juju's domain model, change log inspection, and multi-model query execution.

When you start the REPL, it defaults to the controller database. You can switch between the controller database and model databases at any time using the `.switch` or `.open` commands.

```{warning}
Write operations (INSERT, UPDATE, DELETE) directly modify the controller's state. These operations should only be performed when directed by Juju developers or in emergency recovery scenarios. Always back up your controller before performing write operations.
```

```{note}
For advanced database administration or troubleshooting that requires direct Dqlite cluster access without Juju-specific features, refer to the [Dqlite documentation](https://dqlite.io/docs).
```

## List of `juju_db_repl` commands

(juju_db_repl-change-log)=
### `.change-log`

Show the change log entries in the current database. The change log tracks modifications to database entities.

(juju_db_repl-change-stream)=
### `.change-stream`

Show the entries of the change log that the change stream will view. Change streams are used internally by Juju for real-time updates.

(juju_db_repl-ddl)=
### `.ddl`

Show the Data Definition Language (DDL) statement for the specified table, trigger, or view.

**Usage:** `.ddl <name>`

**Example:**
```text
.ddl application
```

(juju_db_repl-describe-cluster)=
### `.describe-cluster`

Describe the Dqlite cluster configuration, showing node IDs, addresses, and roles (leader/follower).

(juju_db_repl-exit)=
### `.exit`, `.quit`

Exit the REPL and return to the shell.

(juju_db_repl-fk-list)=
### `.fk-list`

List foreign key relationships that reference the specified table and column.

**Usage:** `.fk-list <table> <column> [identifier]`

Without an identifier, shows:
- `child_table`: Table containing the foreign key
- `child_column`: Column in the child table
- `parent_column`: Referenced column in the parent table
- `fk_id`: Foreign key constraint identifier
- `fk_seq`: Sequence number for composite foreign keys

With an identifier (specific UUID/value):
- All the above fields
- `reference_count`: Live count of references in each child table

**Example:**
```text
.fk-list unit uuid
.fk-list unit uuid abc-123-def-456
```

(juju_db_repl-help)=
### `.help`, `.h`

Display the help message with a list of all available commands.

(juju_db_repl-models)=
### `.models`

List all models in the controller with their UUIDs and names.

(juju_db_repl-open)=
### `.open`

Open a specific model database by its UUID. Useful when you have the UUID but not the model name.

**Usage:** `.open <model-uuid>`

**Example:**
```text
.open f5e8a7b2-1234-5678-90ab-cdef12345678
```

(juju_db_repl-query-models)=
### `.query-models`

Execute the same SQL query on all model databases and print the results for each. Useful for gathering information across all models.

**Usage:** `.query-models <query>`

**Example:**
```text
.query-models SELECT COUNT(*) FROM unit
```

(juju_db_repl-switch)=
### `.switch`

Switch to a different database context.

**Usage:**
- `.switch model-<name>` — Switch to the database for the specified model. Use the model name as shown in `.models` output.
- `.switch controller` — Switch back to the controller global database.

**Example:**
```text
.switch model-default
.switch controller
```

(juju_db_repl-tables)=
### `.tables`

List all standard tables in the current database.

(juju_db_repl-triggers)=
### `.triggers`

List all trigger tables in the current database.

(juju_db_repl-views)=
### `.views`

List all views in the current database.
