(script)=
# Script

In Juju, a **script** refers to any script you execute on a {ref}`compute resource <resource-compute>` provisioned by Juju, whether it is a charm {ref}`action <action>` or another kind of script, e.g., a terminal command.


(task)=
## Script task


In Juju, a **task** is the execution of a {ref}`script <script>` on a target {ref}`unit <unit>` (e.g., for actions, via {ref}`command-juju-run`, or, for other arbitrary scripts, via {ref}`command-juju-exec`).

Action tasks are run as defined by the charm author (default: sequentially), whereas tasks related to other scripts are run as set by the charm user (default: parallel).

A group of tasks queued by running an action across one or more units forms an {ref}`operation <operation>`.


(operation)=
## Script operation

In Juju, an **operation** is the group of {ref}`tasks <task>` queued by running a {ref}`script <script>` across one or more {ref}`units <unit>`.

