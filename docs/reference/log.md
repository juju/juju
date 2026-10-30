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

A **log** is a computer-generated record about entities, activities, usage patterns, etc., within a system. In Juju, logs are produced by {ref}`jujuagentd` and keep track of machine and unit agents, models, controllers, etc.

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

The logsink file (`logsink.log`) holds an agent's own logs in aggregated form: each machine agent writes them locally through its log sink, which batches and rotates them. `jujuagentd --log-to-stderr` logs to stderr instead. The agent also streams its buffered records to the controller's logsink endpoint. The {ref}`API server <controller>` stores the received records through its log sink. The `juju debug-log` command reads them back through the API. When the controller has a log forwarding endpoint set, the agent forwards its records to [Loki](https://grafana.com/oss/loki/) instead ({ref}`see telemetry <telemetry>`).

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


(log-change-streams)=
## Log change streams

Log change streams are the changes that an agent follows: the model's logging configuration or the controller's log forwarding endpoint. An agent calls `WatchLoggingConfig` with its own tag; the watcher fires on any change to the model configuration. An agent calls `WatchControllerLokiConfig` (in version 2 of the `Logger` facade) with its own tag to hear that the log forwarding endpoint changed.

(log-calls)=
## Log calls

Log calls are the calls that an {ref}`agent <agent>` makes to read its logging configuration through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): reading the logging configuration and reading the log forwarding endpoint. An agent makes each call with its own tag: no model access level applies.

(log-reading)=
### Reading the logging configuration

Reading the logging configuration returns the model's logging configuration, the value of the model configuration key `logging-config`, for each tag given.

The caller, the rules, and the errors are:

- *Caller:* An agent — a {ref}`machine agent <machine-agent>`, a {ref}`unit agent <unit-agent>`, an application agent, or a model agent — for its own tag. No model access level applies.
- *Rules:*
  - The call takes a list of tags. Each tag in the list gets its own result.
  - The answer is the same for every tag: the model's `logging-config` value.
- *Errors:* The agent receives each in the result for its tag:
  - **`"<tag>" is not a valid tag`**: *Trigger:* A tag that does not parse as a tag. The agent receives it with no error code. *Remediation:* Use the agent's own tag.
  - **`permission denied`**: *Trigger:* A tag that is not the agent's own. The agent receives it with the code `unauthorized access`. *Remediation:* Use the agent's own tag.

(log-reading-forwarding)=
### Reading the log forwarding endpoint

Reading the log forwarding endpoint returns the [Loki](https://grafana.com/oss/loki/) push API endpoint that the machine agents of the models on the controller forward their logs to ({ref}`see telemetry <telemetry>`), with the CA certificate that verifies it, whether to skip verifying that certificate, and an organization ID for a multi-tenant Loki. The call is in version 2 of the `Logger` facade.

The caller, the rules, and the errors are:

- *Caller:* An agent — machine, unit, application, or model — for its own tag. No model access level applies.
- *Rules:*
  - The call takes one tag.
  - The answer names the endpoint, whether to skip verifying the certificate, and the organization ID, and carries the CA certificate when one is set.
- *Errors:* The agent receives each in the result for its tag:
  - **`loki config not found`**: *Trigger:* The controller has no log forwarding endpoint set. The agent receives it with the code `not found`. *Remediation:* Set the endpoint ({ref}`see telemetry <telemetry>`).
  - **`"<tag>" is not a valid tag`** and **`permission denied`**: *Trigger and remediation:* As in {ref}`reading the logging configuration <log-reading>`.
