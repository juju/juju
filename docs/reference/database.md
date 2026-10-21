---
myst:
  html_meta:
    description: "Juju database reference: the Dqlite databases of a controller, the records they keep about changes, sequences, and model databases, and the operations that query, register, number, and delete them."
---

(database)=
# Database

```{ibnote}
See also: {ref}`Juju | Manage the databases <manage-the-databases>`
```

```{versionadded} 4.0
```

The **database** of a {ref}`controller <controller>` is the embedded [Dqlite](https://canonical.com/dqlite) SQL database in which the controller keeps its records. A controller has one database for itself and one for each of its {ref}`models <model>`. Every controller node runs one node of the Dqlite cluster, and in a {ref}`high-availability <high-availability>` setup the nodes replicate the data between them.

(database-schema)=
## Database schema

```{ggarch}
:file: ../juju.ggarch
:view: Change log database schema
:caption: Every database keeps a change log. A change belongs to one edit type and one change namespace, and an edit type and a change namespace each have many changes.
:alt: Three boxes of the same size inside a group named Model database and controller database, each with a filled table mark in the corner. Change is in the middle. Edit type is to its left (m to 1). Change namespace is to its right (m to 1).
```

The database schema is the set of records that the controller database and the model databases keep about the databases themselves. Both kinds of database keep the change log, which is the first four records below. The records, with the rules that apply to each, are:

- **Change:** One create, update, or delete of a row in a watched table (a table whose changes the change log records), with the identifier of the row and the time of the change.
  - *Rules:*
    - A change has an edit type and a change namespace.
    - A change ID is a number that increases with each change.
- **Edit type:** The kind of change: `create`, `update`, or `delete`, stored as the numbers 1, 2, and 4. The numbers are bit values, so that one request can ask for several edit types at once.
  - *Rules:*
    - An edit type appears once.
- **Change namespace:** The name of a kind of change, which is generally the name of the watched table, with a description.
  - *Rules:*
    - A change namespace name appears once.
- **Watermark:** The position of one controller node in the change log: the lowest and the highest change ID that the node has seen, with the time of the last update. Both IDs are -1 until the node has seen a change.
  - *Rules:*
    - A controller node has one watermark.
- **Sequence:** The last number handed out for a sequence namespace (a name such as `machine` or `application_mysql`; see {ref}`allocating sequence numbers <database-allocating-sequence-numbers>`). Only model databases keep sequences.
  - *Rules:*
    - A sequence namespace appears once.
- **Database namespace:** The name of a model database that the controller hosts, which is the model's UUID. Only the controller database keeps them.
  - *Rules:*
    - A database namespace appears once.
- **Staged database deletion:** The name of a model database that is waiting to be deleted, with the time it was staged. Only the controller database keeps them. A staged deletion has no link to a model record, because the model record is already gone when it exists.
  - *Rules:*
    - A database namespace has one staged deletion.

A change is a row in the table `change_log`. Its edit type and change namespace are rows in the lookup tables `change_log_edit_type` and `change_log_namespace`. A watermark is a row in `change_log_witness`. These four tables exist in every database. A sequence is a row in the table `sequence` of a model database. A database namespace is a row in the table `namespace_list` of the controller database. A staged database deletion is a row in the table `model_database_deletion` of the controller database. {ref}`juju-db-repl` shows the definition of any table with `.ddl <name>` and lists the triggers of the current database with `.triggers`.

(database-change-streams)=
## Database change streams

Database change streams are the notifications that tell the workers of a controller that rows of a database have changed. A trigger on each watched table (a table whose changes the change log records) writes a change to the change log when a row is created, updated, or deleted. Each controller node reads the change log and merges the changes that have the same change namespace and row identifier into one. It then sends every subscriber the edit type, the change namespace, and the row identifier. A subscriber reads the database for the current data, because the notification does not contain it.

The change log is pruned. Each controller node records in its watermark how far it has read. About every five seconds, a worker that trims the change log deletes the changes up to the lowest `lower_bound` of all watermarks. When the watermarks lie outside the last ten minutes, it logs the warning `watermarks "<controller ID>" are outside of window, check logs to see if the change stream is keeping up`.

The {ref}`db shell <juju-db-repl>` shows the change log of the current database with two commands:

- `.change-log` lists every change with its ID, edit type, change namespace, row identifier, and time ({ref}`details <juju-db-repl-change-log>`).
- `.change-stream` lists the changes that a subscriber would receive: one for each change namespace and row identifier, with the highest ID ({ref}`details <juju-db-repl-change-stream>`).

(database-operations)=
## Database operations

Database operations are the actions that read a database, number its records, and register or delete a model database. A person with a shell on a controller machine can query the databases. The {ref}`controller agent <controller-agent>` (the process on each controller node that runs the controller's workers) performs the other operations. The agent is the caller of each of them, because no client calls them directly.

(database-querying)=
### Querying databases

```{ibnote}
See also: {ref}`Juju | Manage the databases <manage-the-databases>`
```

Querying databases runs SQL against the controller database or a model database from the {ref}`db shell <juju-db-repl>` on a controller machine.

```{warning}
A write (`INSERT`, `UPDATE`, or `DELETE`) changes the state of the controller directly. Write only when Juju developers direct it or to recover a controller, and back up the controller first.
```

The operation's required access, rules, and errors are:

- *Required access:* A shell on a controller machine, where the `juju_db_repl` command runs.
- *Rules:*
  - The shell starts in the controller database.
  - The `.switch` command takes `controller` or `model-<name>`. The `.open` command takes a model UUID. The `.models` command lists the model databases.
  - The `.query-models` command runs one SQL query on every model database.
- *Errors:* The shell prints each to standard error:
  - **`invalid model namespace name: expected "model-<name>" or "controller"`**: *Trigger:* The `.switch` command got a name that is neither `controller` nor starts with `model-`. *Remediation:* Use `controller` or `model-<name>`.
  - **`model "<name>" not found`**: *Trigger:* The `.switch` command got `model-<name>` and no model has that name. *Remediation:* Run `.models` and use a listed name.

(database-registering)=
### Registering model databases

Registering model databases adds the UUID of a model to the database namespaces of the controller when the model is created. The {ref}`controller agent <controller-agent>` is the caller.

The operation's required access and rules are:

- *Required access:* The controller agent.
- *Rules:*
  - The namespace is the UUID of the model.
  - A UUID that is already registered is refused with `database namespace already registered for model "<uuid>"`.

(database-allocating-sequence-numbers)=
### Allocating sequence numbers

Allocating sequence numbers hands out the next number of a counter that Juju keeps for the names and IDs it assigns in order. Juju allocates the number inside the transaction that needs it.

The operation's required access and rules are:

- *Required access:* The controller agent, in the model database of the model whose record needs the number.
- *Rules:*
  - The first number of a sequence is 0. Each allocation hands out the number one higher than the last.
  - A model keeps one sequence for each of these namespaces: `machine` (machine IDs), `machine_container_<parent machine ID>` (container numbers on a machine), `application_<application name>` (unit numbers of an application), `charm_<charm name>` (revisions of a charm that needs sequencing), `application_charm_<application UUID>` (the modified version of an application's charm), `operation` (operation and task IDs), `relation` (relation IDs), `storage`, `filesystem`, and `volume` (the IDs of storage instances, filesystems, and volumes).
  - The sequence of a namespace is created by its first allocation.

(database-deleting)=
### Deleting model databases

Deleting model databases removes the database of a model from the controller after the model is gone. The {ref}`controller agent <controller-agent>` of each controller node is the caller.

The operation's required access, rules, and errors are:

- *Required access:* The controller agent.
- *Rules:*
  - When a model is {ref}`destroyed <model-destroying>` and its records are removed, the agent deletes its database last.
  - The removal of a model also deletes the model's UUID from the database namespaces, so that the database cannot come back.
  - When a model has {ref}`migrated <model-migrating>` away, the controller stages the deletion of the database in the transaction that purges the model. The agent then deletes the database and removes the staged deletion.
  - A staged deletion is skipped while its namespace is registered again, which happens when the model has returned to the controller.
  - A database that is already gone counts as deleted.
- *Errors:* The agent logs each and retries with a growing delay. The staged deletion stays until the database is deleted:
  - **`deleting database for model "<uuid>": the model no longer resides on this controller, but its database is still present: <reason>`**: *Trigger:* The deletion of a staged database failed. *Remediation:* Read the reason in the controller log and fix its cause.

(database-and-model-migration)=
## Databases and model migration

A migration moves a model's sequences to the target controller, so that numbering continues there. A model that comes from Juju 3.x gets its sequences renamed to the 4.x namespaces: `task` becomes `operation`, `stores` becomes `storage`, `application-<name>` becomes `application_<name>`, and a container sequence such as `machine0lxdContainer` becomes `machine_container_0`. The source controller deletes its database of the model after the migration reaches its last phase (see {ref}`Migrating models <model-migrating>`).
