---
myst:
  html_meta:
    description: "Juju logs reference: agent logs, audit logs, log files structure, verbosity levels, and log management for debugging and monitoring."
---

(log)=
# Log

```{ibnote}
See also: {ref}`manage-logs`
```

A **log** is a computer-generated record about entities, activities, usage patterns, etc., within a system. In Juju, logs are produced by {ref}`jujud` and keep track of machine and unit agents, models, controllers, etc.

## Juju agent logs - machines

In machine deployments, Juju agent logs are organised into a number of files. These files are located on every machine that Juju creates, including the controller. Specifically, they can be found under `/var/log/juju`, and may include:

### Agent log files

Agent log files (e.g., `/var/log/juju/unit-controller-0.log` ) contain the logs for the machine and unit {ref}`agents <agent>`.

### Model log files

Model log files (e.g., `/var/log/juju/models/admin-test-3850c8.log`) contain the logs for all the [workers](https://juju.is/docs/dev/worker) on a {ref}`model <model>`.

### The audit log file

The audit log file (`/var/log/juju/audit.log`) logs all the client commands and all the API calls and errors responses associated with a {ref}`controller <controller>`, classified as one of the following:

-   *Conversation:* A collection of API methods associated with a single top-level CLI command.
-   *Request:* A single API method.
-  *ResponseErrors:* Errors resulting from an API method

The audit log file can be found only on controller machines.

### The logsink log file

The logsink file (`logsink.log`) contains all the agent logs shipped to the {ref}`controller <controller>`, in aggregated form. These logs will end up in Juju's internal database, MongoDB.

```{important}

In a controller high availability scenario, `logsink.log` is not guaranteed to contain all messages since agents have a choice of several controllers to send their logs to. The `debug-log` command should be used for accessing consolidated data across all controllers.

```

(the-machine-lock-log-file)=
### The machine-lock log file

The machine-lock log file (`machine-lock.log`) contains logs for the machine lock. The file is only written written after the lock has been released and its purpose is to give more visibility to who has been holding the machine lock.

The machine lock is a file lock that synchronises hook execution on Juju machines. (A machine will only ever run one {ref}`hook <hook>` at a time.) The lock is used to serialize a number of activities of the agents on the machines started by Juju, as follows:

- The {ref}`machine agent <machine-agent>` will acquire the lock when it needs to install software to create containers, and also in some other instances.

- The {ref}`unit agents <unit-agent>` acquire the machine lock whenever they are going to execute hooks or run actions. Sometimes, when there are multiple units on a given machine, it is not always clear as to why something isn’t happening as soon as you’d normally expect. This log file is to help give you insight into the actions of the agents.

## Juju agent logs - Kubernetes

In Kubernetes deployments, logs are written directly to `stdout` of the container and can be retrieved with native Kubernetes methods: `kubectl logs <pod-name> -n <model-name>` .

By default, it will fetch the logs from the main container `charm` container. When fetching logs from other containers, use additional `-c` flag to specify the container, i.e. `kubectl logs -c <container-name> <pod-name> -n <model-name>` .

