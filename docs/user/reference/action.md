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


