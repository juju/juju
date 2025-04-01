(manage-actions)=
# How to manage actions

<!--
SOURCE: https://discourse.charmhub.io/t/juju-actions-opt-in-to-new-behaviour-from-juju-2-8/2255
TODO: Add more example outputs. (The doc above has many but they're from 2020, so they might not be the latest. And I don't quite get the first bit about the custom-defined action -- how does it get attached to the charm?
-->

> See also: {ref}`action`

This document demonstrates how to manage actions.


## List all actions

To list the actions defined for a deployed application, use the `actions` command followed by the deployed charm's name. For example, assuming you've already deployed the `git` charm, you can find out the actions it supports as below:

```text
juju actions git
```

This should output:

```text
Action            Description
add-repo          Create a git repository.
add-repo-user     Give a user permissions to access a repository.
add-user          Create a new user.
get-repo          Return the repository's path.
list-repo-users   List all users who have access to a repository.
list-repos        List existing git repositories.
list-user-repos   List all the repositories a user has access to.
list-users        List all users.
remove-repo       Remove a git repository.
remove-repo-user  Revoke a user's permissions to access a repository.
remove-user       Remove a user.
```

By passing various options, you can also do a number of other things such as specify a model or an output format or request the full schema for all the actions of an application. Below we demonstrate the `--schema` and `--format` options:

```text
juju actions git --schema --format yaml
```

Partial output:

```text
add-repo:
  additionalProperties: false
  description: Create a git repository.
  properties:
    repo:
      description: Name of the git repository.
      type: string
  required:
  - repo
  title: add-repo
  type: object
```

```{note}

The full schema is under the `properties` key of the root action. Actions rely on [JSON-Schema](http://json-schema.org) for validation. The top-level keys shown for the action (`description` and `properties`) may include future additions to the feature.

```

> See more: {ref}`command-juju-actions`

## Show details about an action

To see detailed information about an application action, use the `show-action` command followed by the name of the charm and the name of the action. For example, the code below will show detailed information about the `backup` action of the `postgresql` application.

```text
juju show-action postgresql backup
```

<!--add sample output-->

> See more: {ref}`command-juju-show-action`


## Run an action

```{important}

**Did you know?** When you run an action, how the action is run depends on the type of the charm. If your charm is a machine charm, actions are executed on the same machine as the application. If your charm is a Kubernetes charm implementing the sidecar pattern, the action is run in the charm container.

```


To run an action on a unit, use the `run` command followed by the name of the unit and the name of the action you want to run. 

```text
juju run mysql/3 backup
```

By using various options, you can choose to run the action in the background, specify a timeout time, pass a list of actions in the form of a YAML file, etc. See the command reference doc for more.

Running an action returns the overall operation ID as well as the individual task ID(s) for each unit.


> See more: {ref}`command-juju-run` (before `juju v.3.0`, `run-action`)

(manage-action-tasks)=
## Manage action tasks
> See also: {ref}`task`

### Show details about a task

To drill down to the result of running an action on a specific unit (the stdout, stderror, log messages, etc.), use the `show-task` command followed by the task ID (returned by the `run` command). For example,

```text
juju show-task 1
```

> See more: {ref}`command-juju-show-task`

### Cancel a task


Suppose you've run an action but would now like to cancel the resulting pending or running task. You can do so using the `cancel-task` command. For example:

```text
juju cancel-task 1
```

> See more: {ref}`command-juju-cancel-task`

(manage-action-operations)=
## Manage action operations
> See also: {ref}`operation`

### View the pending, running, or completed operations


To view the pending, running, or completed status of each `juju run ... <action>` invocation, run the `operations` command:

```text
juju operations
```

This will show the operations corresponding to the actions for all the application units. You can filter this by passing various options (e.g., `--actions backup`, `--units mysql/0`, `--machines 0,1`, `--status pending,completed`, etc.).

> See more: {ref}`command-juju-operations`

### Show details about an operation

To see the status of the individual tasks belonging to a given operation,  run the `show-operation` command followed by the operation ID.

```text
juju show-operation 1
```

As usual, by adding various options, you can specify an output format, choose to watch indefinitely or specify a timeout time, etc.

> See more: {ref}`command-juju-show-operation`

## Debug an action


To debug an action (or more), use the `debug-hooks` command followed by the name of the unit and the name(s) of the action(s). For example, if you want to check the `add-repo` action of the `git` charm, use:

```text
juju debug-hooks git/0 add-repo
```

> See more: {ref}`command-juju-debug-code`, {ref}`command-juju-debug-hooks`
