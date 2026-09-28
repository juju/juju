---
myst:
  html_meta:
    description: "Juju action reference: the charm-declared action definition, the operation and task records that persist a run, the task status vocabulary, execution by owner (enqueue, running, cancellation, watchers), and rules."
---

(action)=
# Action

In Juju, an **action** is an operation that a {ref}`charm <charm>` defines
and a {ref}`user <user>` with the right {ref}`access level
<user-access-levels>` invokes on demand against the charm's
{ref}`application <application>` or one of its {ref}`units <unit>`:
snapshot a database, add a user to a system, dump debug information.
The charm declares each action's name, description and parameter
schema; invoking an action creates an {ref}`operation <operation>` and
one {ref}`task <task>` per target (see {ref}`the action in the data
model <the-action-in-the-data-model>`).

```{ibnote}
See examples: [Charmhub | `kafka` > Actions](https://charmhub.io/kafka/actions), [Charmhub | `prometheus-k8s` > Actions](https://charmhub.io/prometheus-k8s/actions), etc.
```

(the-actions-declaration)=
## Action in the declaration layer

- **The definition:** The charm's `actions.yaml` carries each action's
  name, description, parameter schema, and its parallelism defaults.
  Juju records the definitions when the charm revision lands (the same
  model-write act as the {ref}`charm's <charm>` own); the declaration
  needs no Juju-side access of its own.
- **The invocation:** Running an action against units or the
  application requires model {ref}`write access
  <user-access-model-write>`; running an arbitrary command requires
  model {ref}`admin access <user-access-model-admin>`; cancelling a
  run's tasks requires model write access. The controller-side
  counterpart is the same call over the controller API.

```{ibnote}
See also: {ref}`Juju | Manage actions <manage-actions>`, {ref}`Terraform Provider for Juju | Manage actions <tfjuju:manage-actions>`
```

(the-action-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - The name matches the action-name grammar: lowercase letters and
    digits, hyphens allowed inside (`snapshot-db`, never `-snapshot`
    or `Snapshot`).
  - The name is not reserved: `juju` itself and the `juju-` prefix are
    reserved names.
  - The parallelism defaults (the parallel flag and the execution
    group) come from the charm's definition and become the defaults of
    every operation the action enqueues.
- **Errors:**
  - **`bad action name <name>`:** Triggered when a declared action
    name fails the grammar. Remediation: rename it to lowercase
    letters and digits with hyphens inside.
  - **`cannot use action name <name>`:** Triggered when the name is
    reserved (`juju`, or a `juju-` prefix). Remediation: pick an
    unreserved name.

(the-action-in-the-data-model)=
## Action in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Operation hierarchy
:no-legend:
:caption: The entity hierarchy: an operation groups 1..N tasks (one per receiver); the parallel and execution-group flags live on the operation, shared by all tasks; an action-definition join record exists 1:1 only when the operation is an action (its absence = an exec, modelled as the predefined 'juju-exec' action); each task reports 0..1 status and runs on a unit or machine; results go to the object store.
:alt: Operation record to task record to unit task to unit; operation action record above operation; task status below task.
```

The charm's definitions and Juju's runs are persisted in the
{ref}`model database <data-model-full-spine>` as follows:

- **The operation is the run's record:** its summary, the enqueue/start/complete
  times, and the parallel flag and execution group every task of the
  run inherits; the operation's ID and the tasks' IDs are minted from
  one shared sequence.
- **The charm's definition lives with the charm's own records:** the key, the
  description, the parameter schema, and the parallelism defaults.
- **A one-to-one record ties a run to the charm's action definition:** its
  absence is what makes a run an exec (see {ref}`script <script>`).
- **Each task is one record per receiver:** the task's identity and its
  own enqueue/start/complete times.
- **The receiver joins tie each task to what it runs on:** each task is
  tied through one of the two join records to the unit or the machine
  it runs on.
- **The run's parameters are key/value records:**
  the user-passed parameters for an action (the keys match the charm's
  schema), the command and its timeout for an exec.
- **The task's status is a record of its own** (the status, message and
  update time); the vocabulary is eight values: `error`, `running`,
  `pending`, `failed`, `cancelled`, `completed`, `aborting`, `aborted`.
- **The log lines are timestamped records.**
- **The output record points at the run's results blob** in the
  controller's object store.

(the-action-states)=
### Action states

```{ggarch}
:file: ../juju.ggarch
:view: Action task status
:no-legend:
:caption: The task status as the run unfolds -- the agent starts its task (pending to running); it finishes it (completed, or failed with a message); the user's cancel marks a not-yet-started task cancelled and a running one aborting until the agent kills the charm process and reports it aborted.
:alt: State machine: pending to running on the agent starting the task; running to completed or failed when the agent finishes it; pending to cancelled on cancel-task; running to aborting on cancel-task, aborting to aborted when the process is killed.
```

A task carries one status vocabulary, written by the side that owns
each transition. The enqueue writes `pending`, before any agent has
seen the task. The target's agent starts it (`pending` to `running`:
an atomic update that only a pending task can survive) and finishes it
with a status validated against what an active task may report
(`completed`, `failed`, `aborted`, `cancelled`, `error`). The user's
cancel turns a not-yet-started task `cancelled` and a running one
`aborting`, until the agent kills the charm process and reports
`aborted`. There is no separate operation-level state: the
operation's own status is computed on the fly from its tasks (running
over aborting over pending over error over failed over cancelled over
completed), and its completion time is stamped only when the last
active task finishes. The vocabulary's `error` value has no writer in
the current code: it is an accepted completion status and the cancel
path treats it as terminal, but nothing produces it.

(the-action-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - The statuses are writes gated by the writer, never transitions:
    the enqueue writes `pending`, the agent writes `running` and the
    completion, the cancel writes `cancelled` and `aborting`; a
    completion status must be one the task can legitimately report.
  - A task is never restarted: the start is an atomic update that only
    a `pending` record survives.
  - The operation completes when its last active task does; the
    user-visible status is computed from the tasks at query time,
    never stored.
- **Errors:**
  - **`operation not found`:** Triggered when an operation queries an
    operation ID the model does not have. Remediation: check the
    operation ID and the model.
  - **`task not found`:** Triggered when an operation addresses a task
    the model does not have. Remediation: check the task ID and the
    model.

(the-action-operations)=
## Action in the execution layer

Operations on a run split by owner: the controller enqueues it and
records the results; the target's agent runs the task; the user
cancels it.

### Running an action

```{ggarch}
:file: ../juju.ggarch
:sequence: Action run flow
:no-legend:
:caption: juju run enqueues an operation; the controller records per-unit tasks (pending) and the unit agent's watcher resolves them; the task runs via the charm's dispatch script (action-get/set/fail/log during execution), and finishing stores results in the object store. juju cancel-task moves a running task to aborting; the process is killed and the task reports aborted.
:alt: User calls juju run; client enqueues the operation on the controller; controller records operation and per-unit tasks pending; controller notifies unit agent; agent resolves and starts the task (running); agent runs the charm action with jujuc action commands; on cancel the agent aborts; otherwise results stream back and the task completes.
```

The enqueue path checks its targets' existence, not their life: one
transaction writes the operation, its parameter records, the
action-definition record for an action run, and one `pending` task per
target through its receiver join. A target that no longer resolves
fails its own task; the operation still enqueues for the rest.

Each target's agent picks its task up from its watcher, starts it, and
runs the charm's action in the same execution environment as a
{ref}`hook <hook-execution>`: the {ref}`generic environment variables
<generic-environment-variables>` plus the action-specific ones:

* `JUJU_ACTION_NAME` holds the name of the action.
* `JUJU_ACTION_UUID` holds the UUID of the action.

While it runs, the charm's code reports progress and results with the
action {ref}`hook commands <list-of-hook-commands>`: `action-log`
reports a progress message, `action-get` reads the value of a named
parameter as supplied by the user, `action-set` sets a value in the
results, `action-fail` marks the run failed with a message. Any other
hook command works too; a successful action that used, say,
`relation-set` is followed by a {ref}`relation-changed hook
<hook-relation-changed>` on the affected units.

When the action process exits, the agent finishes the task: the status
write, the log lines, and the output record land in the record set,
and the results blob (the return code, stdout and stderr, and whatever
the action set) goes to the controller's object store under the
task's UUID. Exec runs go through the same machinery as the predefined
`juju-exec` action (parallel by default, the command and its timeout
as the operation's parameters); a run command cannot itself name
`juju-exec` or its legacy alias `juju-run`.

(the-action-cancellation)=
### Cancelling an action

Cancelling turns a pending task `cancelled` and a running one
`aborting`; a task that has already reached a terminal status
(`completed`, `failed`, `aborted`, `cancelled`, `error`) is left
alone. The unit's agent polls its action's status, sees the
`aborting` mark, kills the charm process and reports the task
`aborted` through the same finish path as any completion. A machine's
task has no such poll: the machine's action runner has no abort
surface, and a stale running action is finished as `failed` with the
message `action cancelled` when the machine's runner restarts.

(the-action-watchers)=
### Action watchers

The operation domain's watchable service exposes three surfaces: what
a watcher fires on, not who consumes it:

- **Unit task notifications:** The unit's tasks that became `pending`
  or `aborting`: the unit agent's work queue, and how a running task
  learns it has been cancelled.
- **Machine task notifications:** The machine's tasks that became
  `pending`: the machine agent's queue (there is no `aborting`
  signal on this surface).
- **Task logs:** The task's log lines, for a client streaming
  progress.

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-action-rules-and-errors)=
### Execution rules and errors

- **Rules:**
  - The enqueue checks existence, not life: a target that no longer
    exists fails its own task; the operation still enqueues.
  - Only a `pending` task can start; the atomic record-count guard
    refuses a double start, a restart, and a start after a cancel.
  - A completion status must be in the completion set (`completed`,
    `failed`, `aborted`, `cancelled`, `error`).
  - The machine lock owns the parallelism semantics: parallel tasks
    with no execution group run concurrently (bounded by the runner's
    100-slot limiter); everything else serialises through the machine
    lock, non-parallel tasks in `exec-command`, grouped tasks in
    `exec-command-<group>`.
  - Cancelling leaves terminal tasks untouched.
- **Errors:**
  - **`action not defined for charm <charm> (target unit: <unit>)`:**
    Triggered when the run names an action the charm does not declare.
    Remediation: check the charm's actions.
  - **`task not pending`:** Triggered when starting a task that is not
    pending. Remediation: none; a task cannot start twice.
