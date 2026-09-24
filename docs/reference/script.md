---
myst:
  html_meta:
    description: "Juju script reference: tasks, operations, and script execution on compute resources including actions and terminal commands."
---

(script)=
# Script

In Juju, a **script** refers to any script you execute on a {ref}`compute resource <resource-compute>` provisioned by Juju, whether it is a charm {ref}`action <action>` or another kind of script, e.g., a terminal command.

(task)=
## Script task

```{ggarch}
:file: ../juju.ggarch
:view: Operation hierarchy
:no-legend:
:caption: Topology: The entity hierarchy: an operation groups 1..N tasks (one per receiver); the parallel and execution-group flags live on the operation, shared by all tasks; an operation_action row exists 1:1 only when the operation is an action (its absence = an exec, modelled as the predefined 'juju-exec' action); each task reports 0..1 status and runs on a unit or machine; results go to the object store.
:alt: Operation record to task record to unit task to unit; operation action record above operation; task status below task.
```

In Juju, a **task** is the execution of a {ref}`script <script>` on a target {ref}`unit <unit>` (e.g., for actions, via {ref}`command-juju-run`, or, for other arbitrary scripts, via {ref}`command-juju-exec`).

Action tasks are run as defined by the charm author (default: sequentially), whereas tasks related to other scripts are run as set by the charm user (default: parallel).

A group of tasks queued by running an action across one or more units forms an {ref}`operation <operation>`.

(operation)=
## Script operation

In Juju, an **operation** is the group of {ref}`tasks <task>` queued by running a {ref}`script <script>` across one or more {ref}`units <unit>`.

