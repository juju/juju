(action)=
# Action

> See also: {ref}`manage-actions`

In Juju, an **action** is a script that is triggered via the {ref}`juju-cli` and applied to a {ref}`unit <unit>`. It contains a list of commands defined by a  {ref}`charm <charm>` to allow a {ref}`user <user>` with the right {ref}`access level <user-access-levels>` to interact with an {ref}`application <application>` in ways specific to the application. This may include anything from creating a snapshot of a database, adding a user to a system, dumping debug information, etc.

> See examples: [Charmhub | `kafka` > Actions](https://charmhub.io/kafka/actions), [Charmhub | `prometheus-k8s` > Actions](https://charmhub.io/prometheus-k8s/actions), etc.

<!--UPDATES IN JUJU V.3.0:
https://discourse.charmhub.io/t/new-feature-in-juju-2-8-improved-actions-experience/3182
-->

(Starting with `juju v.3.0`: )
 Actions are identified by integers (instead of [UUIDs^](https://en.wikipedia.org/wiki/Universally_unique_identifier)).

(Starting with `juju v.3.0`: ) Running an action defaults to waiting for the output before returning. This synchronous behaviour allows actions to be easily included in command-line pipelines.

(Starting with `juju v.3.0`: ) The execution of an action is organised into {ref}`tasks <task>` and {ref}`operations <operation>`. (If an action defines a named unit of work -- e.g., back up the database -- that can be executed on selected units, a task is the execution of the action on each target unit, and an operation is the group of tasks queued by running an action across one or more units.)

(action-execution)=
## Action execution

Actions operate in an execution environment similar to a {ref}`hook-execution hook`, with additional environment variables available:

* JUJU_ACTION_NAME holds the name of the action.
* JUJU_ACTION_UUID holds the UUID of the action.

An action is used to perform a named, parameterised operation and report back the results of said operation.
Actions are defined by the charm and invoked by the user (using {ref}`command-juju-run`). The default behaviour is that
the command blocks and wait for the action to complete. During this time any progress messages as reported by the action
are logged to the terminal. When the action completes, the result is printed.
An action result is a map of key values, containing data set by the action as it runs, plus the overall exit code
of the action process itself, and the content of stdout and stderr.

The code used to implement an action can call the following hook commands in addition to {ref}`list-of-hook-commands the others`:
* action-log: to report a progress message
* action-get: to get the value of a named action parameter as supplied by the user
* action-set: to set a value in the action results map
* action-fail: to mark the action as failed along with a failure message

In most cases, an action only has a need to run hook commands such as `config-get` to supplement the configuration passed
in via the action parameters. A action may also commonly use `status-set` to update the unit or application status while
it is running.

### What triggers it?

A charm user invoking the action name from the Juju CLI (`juju run <unit/0> foo`,  `juju run <unit/0> <unit/1> foo`).

### Who gets it?

All the units that the charm user has run the action on.

```{note}
When implementing a hook using {ref}`Ops <ops-ops>`, any hyphens in action names are replaced with underscores
in the corresponding event names.
For example, an action named `snapshot-database` would result in an event named `snapshot_database_action`
being triggered when the action is invoked.
```

```{tip}
The action handler in the charm can in principle cause other events to be fired.
For example:
 - deferred events will trigger before the action.
 - if the action handler updates relation data, a `<endpoint name>_relation_changed` will be emitted afterwards on the affected units.
```
