(action)=
# Action

> See also: {ref}`manage-actions`

Actions are defined by a  {ref}`charm <charm>` to allow a {ref}`user <user>` with the right {ref}`access level <user-access-levels>` to interact with an {ref}`application <application>` in ways specific to the application.
This may include anything from creating a snapshot of a database, adding a user to a system, dumping debug information, etc.

An action is triggered via the {ref}`juju-cli` and applied to one or more {ref}`units <unit>`.
It is run with parameters supplied by the user and records the success/fail status and any results for subsequent perusal.


> See examples: [Charmhub | `kafka` > Actions](https://charmhub.io/kafka/actions), [Charmhub | `prometheus-k8s` > Actions](https://charmhub.io/prometheus-k8s/actions), etc.

The default behaviour is that the {ref}`juju-cli` blocks and waits for the action to complete. This synchronous behaviour allows actions to be easily included in command-line pipelines.
As an action is executing, any progress messages as reported by the action are logged to the terminal. When the action completes, the result is printed.
An action result is a map of key values, containing data set by the action as it runs, plus the overall exit code
of the action process itself, as well as the content of stdout and stderr.

The execution of an action is organised into {ref}`tasks <task>` and {ref}`operations <operation>`.
(If an action defines a named unit of work -- e.g., back up the database -- that can be executed on selected units, a task is the execution of the action on each target unit, and an operation is the group of tasks queued by running an action across one or more units.)

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

<!-- This information should be in Ops docs. It doesn't belong here.
```{note}
When implementing an action using [Ops](https://ops.readthedocs.io/en/latest/), any hyphens in action names are replaced with underscores
in the corresponding event names.
For example, an action named `snapshot-database` would result in an event named `snapshot_database_action`
being triggered when the action is invoked.
```
-->

(action-execution)=
## Action execution

Actions operate in an execution environment similar to a {ref}`hook <hook-execution>`, with additional environment variables available:

* JUJU_ACTION_NAME holds the name of the action.
* JUJU_ACTION_UUID holds the UUID of the action.
