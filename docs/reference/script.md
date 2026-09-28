---
myst:
  html_meta:
    description: "Juju script reference: executing scripts on compute resources -- charm actions and arbitrary terminal commands -- as task/operation records."
---

(script)=
# Script

```{ibnote}
See also: {ref}`manage-actions`
```

In Juju, a **script** refers to any script you execute on a {ref}`compute resource <resource-compute>` provisioned by Juju: a charm {ref}`action <action>` or an arbitrary command. The two halves share the same records and machinery: the targets are {ref}`units <unit>` and {ref}`machines <machine>`, and what Juju persists is the execution, an operation with one {ref}`task <task>` per target (see {ref}`the action in the data model <the-action-in-the-data-model>`).

## Scripts in the declaration layer

You run a script with the {ref}`juju-cli`: `juju run` runs a charm action by name against a unit (model {ref}`write access <user-access-model-write>`), `juju exec` runs an arbitrary command against units, applications, or machines (model {ref}`admin access <user-access-model-admin>`), and `juju cancel-task` cancels the run's tasks (model {ref}`write access <user-access-model-write>`). The controller-side counterpart is the same call over the controller API.

```{ibnote}
See also: {ref}`Terraform Provider for Juju | Manage actions <tfjuju:manage-actions>`
```

(the-scripts-records)=
(the-script-record)=
## Scripts in the persistence layer

A script has no record of its own in the {ref}`model database
<data-model-full-spine>`: what Juju persists is the **execution**. A
script run creates an operation whose parameters carry the command and
its timeout, plus one {ref}`task <task>` per target, the same records
an action run creates (see {ref}`the action in the data model
<the-action-in-the-data-model>`). An exec run is modelled as the
predefined `juju-exec` action; there is no separate script record.

The operation service in the controller performs the writes:
`AddExecOperation` inserts the operation, its parameters as key/value
records (the command and its timeout), and one task record per target,
each linked through a unit-task or machine-task record to the unit or
machine it runs on; for a charm action, `AddActionOperation` also
inserts the one-to-one record tying the operation to the charm's
action definition; `FinishTask` stores the per-task satellites (the
status record, the log records, and the output record pointing at the
results blob in the object store).

A script has no states of its own: the task's status vocabulary is
the action machinery's; the machinery in the execution layer drives
the transitions, the record here stores it (see {ref}`the action
states <the-action-states>`).

(task)=
### Task

In Juju, a **task** is the execution of a {ref}`script <script>` on a target {ref}`unit <unit>` or {ref}`machine <machine>`: a charm action run via {ref}`command-juju-run`, an arbitrary script via {ref}`command-juju-exec`.

Action tasks are run as defined by the charm author (default: sequentially), whereas tasks related to other scripts are run as set by the charm user (default: parallel).

(operation)=
### Operation

In Juju, an **operation** is the group of {ref}`tasks <task>` queued by running a {ref}`script <script>` across one or more {ref}`units <unit>` or machines.

(types-of-script)=
### Types of script

The two kinds are genuinely exclusive, and the data model records the
split as a record-derived kind: a run carries a record tying it to the
charm's action definition only when it is a charm action; an arbitrary
script runs as the predefined `juju-exec` action instead.

#### Charm actions

A **charm action** is a named operation the {ref}`charm <charm>`
defines, with its parameters schema and its parallelism defaults.
The user runs it by name with `juju run` (see {ref}`action <action>`).

#### Arbitrary scripts

An **arbitrary script** is any command the user supplies: run with
`juju exec` against units, applications, or machines, with a wait
timeout (`--wait`), the parallel flag and, optionally, an execution
group.

(the-scripts-machinery)=
## Scripts in the execution layer

A script has no machinery of its own: like an action run, it
executes on the targeted units' agents; the controller only enqueues
the operation and records the results. By default the command waits
for the tasks to finish (`--wait` caps the wait; five minutes by
default); with `--background` it returns as soon as the operation is
enqueued: the record exists, and the work is still unfolding.

The mechanism, the task states and the cancellation story are the
action's: each target's agent picks its task up from its watcher,
runs it, and reports the status and results, and the user cancels
with `juju cancel-task` (see {ref}`the action's operations
<the-action-operations>` and {ref}`cancelling an action
<the-action-cancellation>`). The same watch surfaces serve script
runs; the machine task notifications are the machine agent's queue
(see {ref}`the action watchers <the-action-watchers>`).

(the-script-rules-and-errors)=
## Script rules and errors

A script has no rules of its own: the run's rules, the parallel and
execution-group semantics (the machine lock's) and the validated task
statuses, and its errors (`operation not found`, `task not found`,
`task not pending`) are the action machinery's (see {ref}`the
action's rules and errors <the-action-rules-and-errors>`). The wait,
parallel and execution-group defaults are stated in {ref}`Types of
script <types-of-script>`; the command and its timeout are the
operation's parameters, recorded in the persistence layer.
