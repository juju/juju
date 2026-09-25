---
myst:
  html_meta:
    description: "Juju script reference: executing scripts on compute resources -- charm actions and arbitrary terminal commands -- as task/operation records."
---

(script)=
# Script

In Juju, a **script** refers to any script you execute on a {ref}`compute resource <resource-compute>` provisioned by Juju, whether it is a charm {ref}`action <action>` or another kind of script, e.g., a terminal command.

(the-scripts-records)=
## The script's records

(the-script-record)=
### The script's identity

In the model database, a script is not a record of its own: what Juju
persists is the **execution**. A script run creates an operation whose
parameters carry the command and its timeout, plus one {ref}`task
<task>` per target -- the same records an action run creates (see
{ref}`the action in the data model <the-action-in-the-data-model>`).
An exec run is modelled as the predefined `juju-exec` action; there
is no separate script table.

(task)=
### Task

In Juju, a **task** is the execution of a {ref}`script <script>` on a target {ref}`unit <unit>` (e.g., for actions, via {ref}`command-juju-run`, or, for other arbitrary scripts, via {ref}`command-juju-exec`).

Action tasks are run as defined by the charm author (default: sequentially), whereas tasks related to other scripts are run as set by the charm user (default: parallel).

(operation)=
### Operation

In Juju, an **operation** is the group of {ref}`tasks <task>` queued by running a {ref}`script <script>` across one or more {ref}`units <unit>`.

(types-of-script)=
### Types of script

The two kinds are genuinely exclusive, and the data model records the
split: a run carries an operation-action row tying it to the charm's
action definition only when it is a charm action; an arbitrary script
runs as the predefined `juju-exec` action instead.

#### Charm actions

A **charm action** is a named operation the {ref}`charm <charm>`
defines -- with its parameters schema and its parallelism defaults.
The user runs it by name (see {ref}`action <action>`).

#### Arbitrary scripts

An **arbitrary script** is any command the user supplies: run against
units (via `juju run`) or machines (via `juju exec`), with a timeout,
the parallel flag and, optionally, an execution group.

(the-scripts-machinery)=
## The script's machinery

A script has no machinery of its own -- like an action run, it executes
on the targeted units' agents; the controller only enqueues and
records.

(the-script-operations)=
### Script operations

Running a script enqueues the operation and its tasks exactly as an
action run does -- the mechanism, the task states and the
cancellation story are the {ref}`action's <the-action-operations>`:
the targets' agents pick the tasks up, run them, and report the
status and results; the user cancels with `juju cancel-task`.

(the-script-rules-and-errors)=
## Script rules and errors

- the command and its timeout are stored as the operation's
  parameters;
- action tasks default to sequential (the charm author's choice);
  exec tasks default to parallel (the user's choice) -- both overridable,
  and everything outside a parallel-without-group run serialises
  through the machine lock;
- the run's errors are the action machinery's (see
  {ref}`action rules and errors <the-action-rules-and-errors>`).

(related-entities-script)=
## Entities related to the script

- **Actions** are the charm-defined half of the script family
  (see {ref}`action <action>`).
- **Tasks and operations** are the persisted execution records
  (see {ref}`the action in the data model
  <the-action-in-the-data-model>`).
- **Units and machines** are the targets (see {ref}`unit <unit>`,
  {ref}`machine <machine>`).
