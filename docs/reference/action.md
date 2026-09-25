---
myst:
  html_meta:
    description: "Juju action reference: charm-defined operations for application-specific tasks. The action record, action/task/operation records, task states, running and cancelling actions, watchers, and rules."
---

(action)=
# Action

```{ibnote}
See also: {ref}`manage-actions`
```

Actions are defined by a  {ref}`charm <charm>` to allow a {ref}`user <user>` with the right {ref}`access level <user-access-levels>` to interact with an {ref}`application <application>` in ways specific to the application.
This may include anything from creating a snapshot of a database, adding a user to a system, dumping debug information, etc.

```{ibnote}
See examples: [Charmhub | `kafka` > Actions](https://charmhub.io/kafka/actions), [Charmhub | `prometheus-k8s` > Actions](https://charmhub.io/prometheus-k8s/actions), etc.
```

(the-actions-records)=
## The action's records

(the-action-record)=
### The action's identity

In the model database, an action has two halves: the **definition**,
a record of the {ref}`charm <charm>` -- the action's name, its
description, its parameters schema, and its parallelism defaults --
pair of records Juju creates when the action is executed: the
**operation** (one run across all the targeted units, carrying the
supplied parameters and the parallelism settings) and one **task** per
target (see {ref}`task <task>` and {ref}`operation <operation>`).
Each task carries its own status, log, and results (the results go to
the controller's object store).

An action is triggered via the {ref}`juju-cli` and applied to one or more {ref}`units <unit>`.
It is run with parameters supplied by the user and records the success/fail status and any results for subsequent perusal.

The default behaviour is that the {ref}`juju-cli` blocks and waits for the action to complete. This synchronous behaviour allows actions to be easily included in command-line pipelines.
As an action is executing, any progress messages as reported by the action are logged to the terminal. When the action completes, the result is printed.
An action result is a map of key values, containing data set by the action as it runs, plus the overall exit code
of the action process itself, as well as the content of stdout and stderr.

The code used to implement an action can call any {ref}`hook command <list-of-hook-commands>` as well as the following action commands:
* `action-log`: to report a progress message
* `action-get`: to get the value of a named action parameter as supplied by the user
* `action-set`: to set a value in the action results map
* `action-fail`: to mark the action as failed along with a failure message

```{tip}
In many cases, an action only has a need to run hook commands such as `config-get` to supplement the configuration passed
in via the action parameters. An action may also commonly use `status-set` to update the unit or application status while
it is running.
If the action does use a hook command like `relation-set`, after the action completes successfully, a
{ref}`relation-changed hook <hook-relation-changed>`  will be emitted afterwards on the affected units.
```

(the-action-in-the-data-model)=
### The action in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Operation hierarchy
:no-legend:
:caption: Topology: The entity hierarchy: an operation groups 1..N tasks (one per receiver); the parallel and execution-group flags live on the operation, shared by all tasks; an operation_action row exists 1:1 only when the operation is an action (its absence = an exec, modelled as the predefined 'juju-exec' action); each task reports 0..1 status and runs on a unit or machine; results go to the object store.
:alt: Operation record to task record to unit task to unit; operation action record above operation; task status below task.
```

The run records are: the `operation` (summary, enqueue/start/complete
times, the parallel flag and the execution group, and the parameters
as key/value rows -- for an exec run, the command and its timeout);
the `operation_action` row tying the operation to the charm's action
definition (absent for exec runs); the `operation_task` records, one
per target, each linked through a unit-task or machine-task record to
the unit or machine it runs on; and the per-task satellites -- the
status record, the log rows, and the output record pointing at the
results blob in the object store.

(the-action-states)=
### Action states

A task carries one status vocabulary, written by the side that owns
each transition -- the running agent starts and finishes its tasks,
the user cancels. There is no separate action-level state: the
operation is completed when its last active task is.

```{ggarch}
:file: ../juju.ggarch
:view: Action task status
:no-legend:
:caption: State machine diagram: The task status as the run unfolds -- the agent starts its task (pending to running); it finishes it (completed, or failed with a message); the user's cancel marks a not-yet-started task cancelled and a running one aborting until the agent kills the charm process and reports it aborted.
:alt: State machine: pending to running on the agent starting the task; running to completed or failed when the agent finishes it; pending to cancelled on cancel-task; running to aborting on cancel-task, aborting to aborted when the process is killed.
```

The vocabulary also has an `error` value for a task that failed before
it could run. Status writes are validated -- a completion status must
be one the task can legitimately report -- and the operation's own
query priority reads running over aborting over pending over error
over failed over cancelled over completed.

(types-of-action)=
### Types of action

An action has no subtypes: the charm defines a flat set of named
actions, and Juju adds no kind column. The one split the data model
records is action vs. an arbitrary script: a run carries an
operation-action record only when it runs a charm action -- an exec
run is modelled as the predefined `juju-exec` action instead (see
{ref}`script <script>`).

(the-actions-machinery)=
## The action's machinery

An action has no machinery of its own -- its runs execute on the
targeted units' agents; the controller only enqueues the operation and
records the results.

(the-action-operations)=
### Action operations

(the-action-execution)=
#### Running an action

```{ggarch}
:file: ../juju.ggarch
:sequence: Action run flow
:no-legend:
:caption: Sequence diagram: juju run enqueues an operation; the controller records per-unit tasks (pending) and the unit agent's watcher resolves them; the task runs via the charm's dispatch script (action-get/set/fail/log during execution), and finishing stores results in the object store. juju cancel-task moves a running task to aborting; the process is killed and the task reports aborted.
:alt: User calls juju run; client enqueues the operation on the controller; controller records operation and per-unit tasks pending; controller notifies unit agent; agent resolves and starts the task (running); agent runs the charm action with jujuc action commands; on cancel the agent aborts; otherwise results stream back and the task completes.
```

Running enqueues the operation and its per-target tasks (pending);
each target's agent picks its task up from its watcher, starts it
(running), and runs the charm's action -- the same execution
environment as a {ref}`hook <hook-execution>`, with the
{ref}`generic environment variables <generic-environment-variables>`
plus the action-specific ones:

* `JUJU_ACTION_NAME` holds the name of the action.
* `JUJU_ACTION_UUID` holds the UUID of the action.

When the action process exits, the agent finishes the task: the
results (return code, stdout, stderr, and whatever the action set) go
to the object store, and when the last active task finishes, the
operation completes. Exec runs go through the same machinery -- the
command and its timeout are stored as the operation's parameters.

(the-action-cancellation)=
#### Cancelling an action

Cancelling (for example, `juju cancel-task`) marks a pending task
cancelled; a running task is marked aborting and its agent kills the
charm process, reporting the task aborted.

(the-action-watchers)=
### Action watchers

The operation domain's watchable service exposes these watch
surfaces:

- **Unit task notifications** -- fires on a task becoming pending or
  aborting: this is the unit agent's work queue, and how a running
  task learns it has been cancelled.
- **Machine task notifications** -- the same, for tasks running
  directly on machines (the machine agent's queue).
- **Task logs** -- the task's log lines, for a client streaming
  progress.

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

(the-action-rules-and-errors)=
## Action rules and errors

The rules an **action definition** must satisfy:

- the action's name matches the action-name pattern and is not one of
  the reserved names;
- the parallel and execution-group settings parse into the operation's
  defaults.

The rules a **task run** must satisfy:

- a task can only be started while it is pending (`task not pending`)
  -- the agent cannot double-start or restart one;
- the parallel and execution-group semantics are the machine lock's:
  parallel tasks with no execution group run concurrently; everything
  else serialises through the machine lock, non-parallel tasks in
  `exec-command`, grouped tasks in `exec-command-<group>`;
- completion statuses are validated against the statuses an active
  task may report.

The errors that encode them:

- *Existence*: `operation not found`, `task not found`,
  `action not defined` (the charm has no such action).
- *State*: `task not pending`.

(related-entities-action)=
## Entities related to the action

- **Charms** define the actions: the definition record is the charm's,
  with its parameters schema and parallelism defaults
  (see {ref}`charm <charm>`).
- **Units and machines** run the tasks -- one task per target, picked
  up by the target's agent (see {ref}`unit <unit>`,
  {ref}`machine <machine>`).
- **Scripts** are the other kind of run: an exec operation executing
  an arbitrary command instead of a charm-defined action (see
  {ref}`script <script>`).
- **Hooks** share the execution environment -- and an action that
  touches relation data emits relation hooks after it completes
  (see {ref}`hook execution <hook-execution>`).
- **Status** can be updated by the running action, the same
  `status-set` path hooks use (see
  {ref}`the application's status <the-application-states>`).
