---
myst:
  html_meta:
    description: "Juju action reference: what an action is, the operations and tasks that record its runs, the status of a task, and the operations that list, run, show, cancel, and prune them."
---

(action)=
# Action

```{ibnote}
See also: {ref}`Juju | Manage actions <manage-actions>`
```

An **action** is a named piece of work, with parameters, that a {ref}`charm <charm>` defines for an {ref}`application <application>` and that a {ref}`user <user>` with the right {ref}`access level <user-access-levels>` can run on the application's {ref}`units <unit>`. Running an action creates an **operation**, the group of **tasks** that run the action, one on each target unit. A task records its status, results, and log messages on the {ref}`controller <controller>`.

```{ibnote}
See examples: [Charmhub | `kafka` > Actions](https://charmhub.io/kafka/actions), [Charmhub | `prometheus-k8s` > Actions](https://charmhub.io/prometheus-k8s/actions), etc.
```

(action-database-schema)=
## Action database schema

```{ggarch}
:file: ../juju.ggarch
:view: Action database schema
:caption: The model database keeps one record for each operation. An operation has many parameters (a parameter belongs to one operation) and many tasks (a task belongs to one operation). An operation that runs an action is linked to the action as the charm declares it. A command run has no such link. A task is linked to one unit or to one machine through a link record that the figure leaves out, so a unit or a machine can have many tasks. A task has one status, which names one of the allowed status values (a value is held by many statuses). A task also has many log messages (a message belongs to one task). The unit, machine, and action are described on their own pages and the figure shows them only for context: all boxes are records, and the three with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Nine boxes of the same size inside a group named Model database. Six of them (Operation, Operation parameter, Task, Task status, Task status value, and Task log message) have a filled table mark in the corner. Operation is on the left. Operation parameter is above it (1 to m), Action is below it (m to 1, optional), and Task is to its right (1 to m). Task status is above Task (1 to 1) and Task status value is to the right of Task status (m to 1). Task log message is below Task (1 to m). Unit is to the right of Task and Machine is below Unit; each is joined to Task (1 to m, one of the two for each task). Unit, Machine, and Action have an outlined arrow mark for records that other pages describe.
```

The action database schema is the set of records that the {ref}`model database <database>` keeps about operations and their tasks. The same records hold commands that a user runs on units or {ref}`machines <machine>`: a command run is an operation with no link to a declared action. The records, with the rules that apply to each, are:

- **Operation:** One run of an action or of a command on one or more targets. It has a number that identifies it, a summary, the times it was enqueued, started, and completed, whether its tasks may run in parallel, and an execution group (a name that makes the tasks of one group run one at a time).
  - *Rules:*
    - An operation number is used once.
    - An operation has at most one link to a declared action. An operation with the link runs that action. An operation without it runs a command.
- **Operation parameter:** One key and value of an operation. For an action these are the parameters the user passed. For a command run these are `command` and `timeout`.
  - *Rules:*
    - An operation has each key once.
    - A parameter can be inserted and deleted, and never updated.
- **Task:** The run of an operation on one target. It has a number that identifies it and the times it was enqueued, started, and completed. Its output, when it has any, is stored as a file in the model's object store (the store where the model keeps files), and the task refers to that file by its path.
  - *Rules:*
    - A task number is used once.
    - A task is linked to a unit or to a machine, never to both. The link is never updated.
    - A task has at most one output.
- **Task status:** The current status of a task, with a message and the time of the last change. A new task has the status `pending`.
  - *Rules:*
    - A task has at most one status.
    - The status is one of the task status values.
- **Task status value:** The allowed statuses: `pending`, `running`, `aborting`, `completed`, `failed`, `cancelled`, `aborted`, and `error`. A task is active while it is `pending`, `running`, or `aborting`. Every other status is final. {ref}`Canceling action tasks <action-cancelling>` and {ref}`Running action tasks <action-execution>` change the status.
- **Task log message:** One progress message of a task, with the time it was recorded.
- **Unit, machine, and charm action:** The records at the far ends of a task and an operation. Other pages own them: see {ref}`unit <unit>`, {ref}`machine <machine>`, and {ref}`charm <charm>`.

(action-change-streams)=
## Action change streams

Action change streams are the notifications about tasks. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). The streams are:

- **Task log messages.** A client calls `WatchActionsProgress` with the tag of a task to hear each new log message of that task. Each message arrives as a JSON string with the fields `message` and `timestamp`. The call needs a logged-in client and checks no model access. A tag that is not an action tag fails with the parse error and no code. A task that does not exist fails with the code `not found`.
- **Tasks of a unit.** The unit agent calls `WatchActionNotifications` to hear the number of each task of its unit that becomes pending or aborting.
- **Tasks of a machine.** The machine agent calls `WatchActionNotifications` of the `MachineActions` facade to hear the number of each task of its machine that becomes pending.

(action-operations)=
## Action operations

Action operations are the calls that {ref}`clients <client>`, {ref}`agents <agent>`, and hook tools make through the controller API (the set of calls that clients and agents make to the controller). A client works with actions, tasks, and operations. The calls of a client belong to the `Action` facade (a named group of API calls). An {ref}`agent <agent>` is a unit agent, the process that runs a unit's charm, or a machine agent, the process that runs commands on a machine. Each logs in as its own unit or machine and acts only for it, so no model access level or block applies to its calls. An agent runs the tasks that target it. A hook tool gives the action code its parameters and takes its results. The controller also prunes old operations on its own.

(action-listing)=
### Listing actions

```{ibnote}
See also: {ref}`Juju | Manage actions <manage-actions>`
```

Listing actions lets a client read the actions that the charm of each named application defines, each with its description and its parameter schema.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model.
- *Rules:*
  - The call takes a list of applications. Each application in the list gets its own result.
- *Errors:* The client receives each in the result for the application, except where stated:
  - **`permission denied`**: *Trigger:* The caller lacks write access. This fails the whole call. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`id not found`**: *Trigger:* The tag of the application is not an application tag. The client receives it with the code `not found`. *Remediation:* Use the form `application-<name>`.
  - **`application "<name>" not found`**: *Trigger:* The model has no application of that name. The client receives it with the code `not found`. *Remediation:* Name an existing application.

(action-running)=
### Running actions

```{ibnote}
See also: {ref}`Juju | Run an action <manage-actions>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Running actions
:caption: Running an action goes in this order. The client asks the controller to enqueue the action on one or more units of one application. The controller stores one operation and one pending task for each unit, then answers at once with the operation and task numbers. The unit agent of each unit hears of its pending task, gets it (a task that is no longer pending is refused), marks it running, and runs the charm's action code with the parameters. The action code reports progress messages to the agent as it goes, and results and a failure message when it ends. The agent sends each progress message to the controller at once, and sends the status, results, and message when the action ends. The client does not wait for the run.
:alt: A sequence diagram with four participants: the client, the controller, the unit agent, and the charm action code. The client asks the controller to enqueue the action on a unit. The controller stores the operation and one pending task for each unit, and returns the operation and task numbers to the client. The controller then notifies the unit agent of the pending task. The unit agent gets the task and marks it running, then runs the action code with its parameters. The action code returns progress messages, results, and failure to the unit agent, which logs progress and then records status, results, and message on the controller.
```

Running actions lets a client queue one action on one or more units of one application. The controller returns the number of the operation and the number of the task of each unit. The run itself goes on in the background, on the {ref}`unit agent <unit-agent>` of each unit. A target is a unit, or the leader unit of an application written as `<application>/leader` (the {ref}`leader unit <leader-unit>` at that moment). A tag is the typed name of an entity, such as `unit-mysql-0`.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model.
- *Rules:*
  - The request must name at least one action.
  - The actions of a request differ only in their receiver. They have the same name, parallel flag, execution group, and parameters. The check compares the first two actions of the request.
  - All receivers are units of one application.
  - The charm of the application must define the action.
  - The parameters are stored as given.
  - The operation summary is `action "<name>"`.
  - A receiver that is not a valid unit tag, a unit that does not exist, or a leader that cannot be found fails alone. The other receivers still get their tasks.
- *Errors:* The client receives each as the error of the call, except where stated, and with no code unless one is given:
  - **`permission denied`**: *Trigger:* The caller lacks write access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`no actions specified`**: *Trigger:* The request names no action. *Remediation:* Name at least one action.
  - **`action name mismatch: "<a>" != "<b>"`**, **`parallel mismatch: <a> != <b>`**, **`execution group mismatch: <a> != <b>`**, and **`parameters mismatch: <a> != <b>`**: *Trigger:* The first two actions differ in that field. *Remediation:* Send one request for each set of values.
  - **`actions must be run on units from the same application`**: *Trigger:* The receivers belong to more than one application. *Remediation:* Send one request for each application.
  - **`action "<name>" not defined for unit "<unit>".`**: *Trigger:* The charm defines other actions but not this one. The client receives it with the code `not found`. *Remediation:* {ref}`List the actions <action-listing>` of the application and use one of them.
  - **`no actions defined for charm <charm>.`**: *Trigger:* The charm defines no action. The client receives it with the code `not found`. *Remediation:* Run the action of a charm that defines it.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid unit tag`**: *Trigger:* A receiver is not a unit tag. The client receives them in the result for the receiver. *Remediation:* Use `unit-<application>-<number>`, or `<application>/leader`.
  - **`linking task to unit: getting unit UUID for "<unit>": unit "<unit>" not found`**: *Trigger:* The unit does not exist. The client receives it in the result for the receiver. *Remediation:* Name an existing unit.
  - **`adding action operation: adding action operation: getting charm UUID for application "<name>": application "<name>" not found`**: *Trigger:* The application of the first unit does not exist. *Remediation:* Name the units of an existing application.

(action-running-commands)=
### Running commands

```{ibnote}
See also: {ref}`Juju | Run an action <manage-actions>`
```

Running commands lets a client queue one shell command as an operation on machines, on units, on all units of applications, or on the leader units of applications, with one task for each target. The command is recorded as an operation with the summary `exec "<command>"`. Running commands also queues the command on all machines of a machine model.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`admin access <user-access-model-admin>` to the model. The change block (an operator setting that makes the controller refuse the calls that change the model; a refused call fails with an "operation blocked" error that carries the operator's message) must not be on.
- *Rules:*
  - A command that contains `juju-exec` or `juju-run` is refused.
  - The operation has the parameters `command` and `timeout`. The timeout is in nanoseconds. The value zero sets no limit.
  - The call on all machines queues one task for each alive machine. It needs a machine model.
  - A target that does not exist fails alone. The other targets still get their tasks.
- *Errors:* The client receives each as the error of the call, except where stated, and with no code unless one is given:
  - **`permission denied`**: *Trigger:* The caller lacks admin access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **The operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Ask the operator to lift the block.
  - **`cannot use "<commands>" as an action command`**: *Trigger:* The command contains `juju-exec` or `juju-run`. The client receives it with the code `not supported`. *Remediation:* Run the command without calling `juju-exec`.
  - **`cannot run on all machines with a <type> model`**: *Trigger:* The model is not a machine model. *Remediation:* Name the units as targets.
  - **`starting exec operation: adding exec operation: getting units for applications [<names>]: no application found`**: *Trigger:* No unit exists for the named applications. *Remediation:* Name an application that has units.
  - **`getting machine UUID for "<name>": machine "<name>" not found`**: *Trigger:* The machine does not exist. The client receives it in the result for the machine. *Remediation:* Name an existing machine.
  - **`linking task to unit: getting unit UUID for "<unit>": unit "<unit>" not found`**: *Trigger:* The unit does not exist. The client receives it in the result for the unit. *Remediation:* Name an existing unit.

(action-showing-tasks)=
### Showing action tasks

```{ibnote}
See also: {ref}`Juju | Show details about a task <manage-action-tasks>`
```

Showing action tasks lets a client read, for each task tag, the receiver, the action name, the parameters, the parallel flag and execution group, the times it was enqueued, started, and completed, the status, the message, the log messages, and the output. The output is a map that holds what `action-set` stored, and for a command the output that {ref}`Running action tasks <action-execution>` describes.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model.
- *Rules:*
  - The call takes a list of tasks. Each task in the list gets its own result.
  - A task that fails to load gets the same error as a bad tag.
- *Errors:* The client receives each in the result for the task, except where stated:
  - **`permission denied`**: *Trigger:* The caller lacks read access. This fails the whole call. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`id not found`**: *Trigger:* The tag is not an action tag, or no task has that number. The client receives it with the code `not found`. *Remediation:* {ref}`List the action operations <action-listing-operations>` and use the tag of one of their tasks.

(action-cancelling)=
### Canceling action tasks

```{ibnote}
See also: {ref}`Juju | Cancel a task <manage-action-tasks>`
```

(action-task-status)=
```{ggarch}
:file: ../juju.ggarch
:view: Task status
:caption: A task starts as `pending`. A pending task becomes `running` when its agent starts it. It becomes `cancelled` when a client cancels it. A running task becomes `aborting` when a client cancels it. A running or aborting task becomes `completed` or `failed` when the agent reports the result. An aborting task becomes `aborted` when the agent reports that it stopped. Recording a result stores the reported status whatever the current status is, and accepts `completed`, `failed`, `cancelled`, `aborted`, and `error`. The figure shows the changes that canceling and the unit agent make.
:alt: A state diagram with seven states. Pending goes to running and to cancelled. Running goes to aborting, to completed, and to failed. Aborting goes to aborted, to completed, and to failed.
```

Canceling action tasks lets a client stop tasks that have not finished and returns each task with its new status.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model.
- *Rules:*
  - The call sets the status of a task as follows:
    - A `pending` task becomes `cancelled`, so it never runs.
    - A `running` task becomes `aborting`. When the unit agent sees that status, it stops the running action by killing its process. The action code is not told of the cancel, so it cannot run cleanup before the process ends.
    - A task with a final status keeps its status. The code leaves a task that is already `aborting` unchanged too.
  - The call takes a list of tasks. Each task in the list gets its own result.
- *Errors:* The client receives each in the result for the task, except where stated:
  - **`permission denied`**: *Trigger:* The caller lacks write access. This fails the whole call. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`id not found`**: *Trigger:* The tag is not an action tag. The client receives it with the code `not found`. *Remediation:* Use the form `action-<number>`.
  - **`action <number> not found`**: *Trigger:* No task has that number. The client receives it with the code `not found`. *Remediation:* {ref}`List the action operations <action-listing-operations>` and use the tag of one of their tasks.

(action-listing-operations)=
### Listing action operations

```{ibnote}
See also: {ref}`Juju | View the pending, running, or completed operations <manage-action-operations>`
```

Listing action operations lets a client read operations in the order of their numbers, each with its summary, times, derived status, and the result of each of its tasks, in the form of showing an action task.

The status of an operation has no record of its own. Each time the operation is read, it is the first of `running`, `aborting`, `pending`, `error`, `failed`, `cancelled`, and `completed` that any of its tasks has. An operation with no tasks has the status `error`. An operation whose tasks are all `aborted` has the status `pending`, because `aborted` is not in that order.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model.
- *Rules:*
  - A request can filter by action names, applications, machines, units, and task statuses. With no filter, all operations return.
  - An operation matches the applications, machines, and units of a filter when any of its tasks is on any of them. It matches the statuses when any of its tasks has any of them.
  - A page holds at most 50 operations. A request can ask for fewer with a limit, and for a later page with an offset.
  - The answer says whether it is truncated, which means that more operations match.
- *Errors:* The client receives each as the error of the call:
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.

(action-showing-operations)=
### Showing action operations

```{ibnote}
See also: {ref}`Juju | Show details about an operation <manage-action-operations>`
```

Showing action operations lets a client read the operations with the numbers in the request, in the same form as listing them, with the same derived status.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model.
- *Rules:*
  - The call takes a list of operations. Each operation in the list gets its own result.
- *Errors:* The client receives each in the result for the operation, except where stated:
  - **`permission denied`**: *Trigger:* The caller lacks read access. This fails the whole call. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid operation tag`**: *Trigger:* The tag is not an operation tag. The client receives them with no code. *Remediation:* Use the form `operation-<number>`.
  - **`operation <number> not found`**: *Trigger:* No operation has that number. The client receives it with no code. *Remediation:* {ref}`List the action operations <action-listing-operations>` and use one of their numbers.

(action-execution)=
### Running action tasks

Running action tasks lets the unit agent of a unit or the machine agent of a machine run the pending tasks that target it. The unit agent calls the `Uniter` facade and runs the charm's action code. The machine agent calls the `MachineActions` facade and runs commands. The status changes that an agent makes are in the {ref}`figure of canceling action tasks <action-task-status>`.

The caller, the rules, and the errors are:

- *Caller:* A unit agent, for its own unit, or a machine agent, for its own machine.
- *Rules:*
  - Getting a task returns its action name, parameters, parallel flag, and execution group while the task is `pending`.
  - The unit agent calls `ActionStatus` to read only the status of a task. This shows it that a running task became `aborting`.
  - The machine agent calls `RunningActions` to list the numbers of the running tasks of its machine.
  - The agent sets the status of a task as follows:
    - Starting a task sets its start time and its status to `running`. A task can start only while it is `pending`.
    - Finishing a task stores the status, the message, and the results. The status must be `completed`, `failed`, `cancelled`, `aborted`, or `error`. The results are stored as a file in the object store when there are any. An aborted task keeps the results that its action code stored before the agent killed its process. The operation gets its completion time when none of its tasks is active.
    - The unit agent reports `completed`. It reports `failed` when the action code called `action-fail` or ended with an error. It reports `aborted` instead of `failed` when the task was `aborting`. If the changes that the action requested cannot be committed, it reports `failed` and appends the error to `stderr`.
  - A task that is not parallel, or that has an execution group, waits for the machine lock of its target. Tasks of one execution group share one lock, so they run one at a time.
  - Logging a message appends it to the log of the task.
  - The unit agent runs an action in the hook environment, so every {ref}`hook command <list-of-hook-commands>` is available. The environment has the {ref}`generic environment variables <generic-environment-variables>` and these:
    - `JUJU_ACTION_NAME`: the name of the action.
    - `JUJU_ACTION_UUID`: the number of the task.
    - `JUJU_ACTION_TAG`: the tag of the task.
  - The output of a command holds `return-code`, `stdout`, and `stderr`. Output that is not valid UTF-8 is encoded in base64, with `stdout-encoding` or `stderr-encoding` set to `base64`.
  - A command with a timeout other than zero is stopped when the timeout ends.
  - A unit agent that was interrupted during an action fails that action with the message `action terminated` when it restarts.
  - A machine agent fails every task of its machine that was running when it started, with the message `action cancelled`. It runs at most 100 commands at once.
- *Errors:* The agent receives each in the result for the task:
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid action tag`**: *Trigger:* The tag is not an action tag. The agent receives them with no code.
  - **`getting receiver for task "<number>": task "<number>": task not found`**: *Trigger:* No task has that number. The agent receives it with no code.
  - **`permission denied`**: *Trigger:* The task targets another unit or machine. The agent receives it with the code `unauthorized access`.
  - **`unit <name> not found`** and **`machine "<name>" not found`**: *Trigger:* A watch names a unit or machine that does not exist. The agent receives them with the code `not found`.
  - **`id not found`**: *Trigger:* The machine agent named a tag that is not a machine tag. The agent receives it with the code `not found`.
  - **`action no longer available`**: *Trigger:* The agent got a task that is no longer pending. The agent receives it with that code.
  - **`starting task "<number>": task "<number>": task not pending`**: *Trigger:* The agent started a task that is no longer pending. The agent receives it with no code.
  - **`Status is not valid for completed tasks`**: *Trigger:* The reported status is not final. The agent receives it with no code.

(action-getting-parameters)=
### Getting action parameters

Getting action parameters lets the action code read the parameters of its task through the hook tool {ref}`action-get <hook-command-action-get>`.

The caller and the rules are:

- *Caller:* The action code of a running task, through the unit agent for its own unit.
- *Rules:*
  - The tool prints the value at a path of keys joined with periods, or all parameters when it has no path.

(action-setting-results)=
### Setting action results

Setting action results lets the action code add values to the results of its task through the hook tool {ref}`action-set <hook-command-action-set>`.

The caller and the rules are:

- *Caller:* The action code of a running task, through the unit agent for its own unit.
- *Rules:*
  - A key is a path of keys joined with periods. Each key is made of lowercase letters, digits, and hyphens, and starts and ends with a letter or digit. The keys `stdout`, `stdout-encoding`, `stderr`, and `stderr-encoding` are reserved. Every value is a string.

(action-failing)=
### Setting action failure

Setting action failure lets the action code mark its task as failed through the hook tool {ref}`action-fail <hook-command-action-fail>`.

The caller and the rules are:

- *Caller:* The action code of a running task, through the unit agent for its own unit.
- *Rules:*
  - Without a message, the message is `action failed without reason given, check action for errors`.

(action-logging)=
### Logging action progress

Logging action progress lets the action code record a progress message of its task through the hook tool {ref}`action-log <hook-command-action-log>`. The unit agent sends the message to the controller at once.

The caller and the rules are:

- *Caller:* The action code of a running task, through the unit agent for its own unit.
- *Rules:*
  - A call without a message fails with `no message specified`.

(action-pruning)=
### Pruning action operations

Pruning action operations deletes old operations with their parameters, tasks, statuses, and log messages, and the stored results of their tasks. The controller does this on its own for each model, once about every 24 hours (a random interval between 12 and 36 hours), and again when the model configuration key {ref}`max-action-results-age <model-config-max-action-results-age>` or {ref}`max-action-results-size <model-config-max-action-results-size>` changes.

The rules are:

- *Rules:*
  - An operation that completed longer ago than `max-action-results-age` is deleted. An age of zero turns this off. Only completed operations are deleted by age.
  - When the stored operations are larger than `max-action-results-size`, the oldest are deleted, those that completed first, as many as the excess divided by the average size of an operation. A size of zero turns this off.

## Actions and model migration

When a {ref}`model <model>` is migrated to another controller, its operations and tasks move, with two exceptions: a task with an empty or malformed receiver name is skipped with a warning; a task whose unit or machine does not exist on the target controller fails the import. See {ref}`Migrating models <model-migrating>`.
