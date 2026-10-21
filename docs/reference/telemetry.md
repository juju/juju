---
myst:
  html_meta:
    description: "Juju telemetry reference: the business metrics that a model sends to Charmhub, the tracing configuration for the controller and its agents, and the log forwarding endpoint."
---

(telemetry)=
# Telemetry

Telemetry is the data that Juju itself sends out about a deployment. It covers two independent things. The {ref}`business metrics <telemetry-business-metrics>` are counts that each model sends to Charmhub. The {ref}`tracing <telemetry-tracing>` and the log forwarding configuration tell the agents and the charms of every model where to send their traces and their logs: the {ref}`controller database <database>` holds them, a caller sets them over the controller's control socket, and the agents receive them through watchers.

(telemetry-business-metrics)=
## Business metrics

Business metrics are the counts that describe a deployment, which the developers of Juju use to improve it. A worker of each model gathers them and sends them to [Charmhub](https://charmhub.io/) once a day by default, together with the model's check for new charm revisions (the agent configuration sets the period). The metrics carry no user information:

- For the controller: the version of Juju that runs it and the controller's UUID.
- For the model: the model's UUID, its cloud, the cloud's type, the cloud's region, and the counts of its applications, units, and machines.
- For each application that comes from Charmhub: the number of its units.

The {ref}`disable-telemetry <model-config-disable-telemetry>` model configuration key turns the metrics off. The check for new charm revisions keeps running either way, without the metrics. The {ref}`transmit-vendor-metrics <model-config-transmit-vendor-metrics>` model configuration key has no effect.

(telemetry-database-schema)=
## Telemetry database schema

The telemetry database schema is the set of records that the {ref}`controller database <database>` keeps about the tracing and log forwarding configuration. Each record holds one configuration. No record links to another. The records, with the rules that apply to each, are:

- **Workload tracing configuration:** The configuration of the tracing that Juju itself exports: the traces of the controller and of the agents of every model on it. Its fields are `http-endpoint` and `grpc-endpoint` (the endpoints of the [OpenTelemetry](https://opentelemetry.io/) collector), `ca-certificate`, `insecure-skip-verify`, `stack-traces`, `sample-ratio`, and `tail-sampling-threshold`.
  - *Rules:*
    - A field appears once.
- **Charm tracing configuration:** The configuration that gives the charms of every model on the controller a collector to write spans (the traced operations of the charm) to. Its fields are `http-endpoint`, `grpc-endpoint`, and `ca-certificate`.
  - *Rules:*
    - A field appears once.
- **Log forwarding configuration:** The endpoint of the [Loki](https://grafana.com/oss/loki/) push API that the machine agents of the models forward their logs to, with the CA certificate that verifies it, whether to skip verifying that certificate, and an organization ID for a multi-tenant Loki.
  - *Rules:*
    - The controller keeps at most one log forwarding configuration.

(telemetry-change-streams)=
## Telemetry change streams

Telemetry change streams are the notifications that tell the agents of a model that the tracing or the log forwarding configuration has changed. Each agent watches the configurations for its own tag: an agent calls `WatchControllerTracingConfig` to hear that the workload tracing configuration changed, and `WatchControllerLokiConfig` to hear that the log forwarding configuration changed. A {ref}`unit agent <unit-agent>` reads the charm tracing configuration when it builds the context of its unit, and passes it to the charm in the environment variables `JUJU_CHARM_TRACE_CONFIG_HTTP`, `JUJU_CHARM_TRACE_CONFIG_GRPC`, and `JUJU_CHARM_TRACE_CONFIG_CA_CERT`, so the charm needs no integration with the collector.

(telemetry-operations)=
## Telemetry operations

Telemetry operations are the actions that set and read the tracing and the log forwarding configuration. The caller that sets them is the user the {ref}`controller machine agent <controller-agent>` runs as, on the controller machine: the configurations are set over the controller's control socket, a Unix socket named `control.socket` in the data directory of the controller machine agent, which accepts JSON `POST` requests. The `juju-controller` charm is the usual caller. The agents of the models are the readers.

(telemetry-tracing)=
### Setting the tracing configuration

Setting the tracing configuration stores the workload tracing configuration and the charm tracing configuration. A field that the request omits or leaves empty is removed from the configuration. Setting an endpoint turns that tracing on; removing both endpoints turns it off.

The operation's required access, rules, and errors are:

- *Required access:* The user the controller machine agent runs as, on the controller machine. No model access level applies.
- *Rules:*
  - The workload tracing configuration is set with a `POST` request to `/workload-tracing-config`, with the fields `http_endpoint`, `grpc_endpoint`, `ca_cert`, `stack_traces`, `sample_ratio`, `tail_sampling_threshold`, and `insecure_skip_verify`.
  - The charm tracing configuration is set with a `POST` request to `/charm-tracing-config`, with the fields `http_endpoint`, `grpc_endpoint`, and `ca_cert`.
  - The sample ratio is a fraction between 0 and 1.
  - The tail sampling threshold is a duration, such as `1ms`.
- *Errors:* The socket answers each with the status named in the entry and this text in the body:
  - **`invalid workload tracing config: sample-ratio value <value> must be a ratio between 0 and 1`** (400): *Trigger:* The sample ratio is below 0 or above 1. *Remediation:* Send a fraction between 0 and 1.
  - **`invalid workload tracing config: tail-sampling-threshold value "<value>" must be a valid duration`** (400): *Trigger:* The threshold is not a duration. *Remediation:* Send a duration such as `1ms`.
  - **`invalid workload tracing config: tail-sampling-threshold value "<value>" must be a positive duration`** (400): *Trigger:* The threshold is negative. *Remediation:* Send a non-negative duration.
  - **`request Content-Type must be application/json`** (415): *Trigger:* The request has no `Content-Type: application/json` header. *Remediation:* Send the header.
  - **`missing request body`** and **`request body is not valid JSON: <reason>`** (400): *Trigger:* The request has no body, or a body that is not JSON. *Remediation:* Send a JSON body.
  - **`request body must not exceed 1048576 bytes`** (413): *Trigger:* The body is over 1 MiB. *Remediation:* Send a smaller body.

### Setting the log forwarding endpoint

Setting the log forwarding endpoint stores where the machine agents of the models send their logs. Removing it stops the forwarding.

The operation's required access, rules, and errors are:

- *Required access:* The user the controller machine agent runs as, on the controller machine. No model access level applies.
- *Rules:*
  - The endpoint is set with a `POST` request to `/loki-endpoint`, with the fields `url`, `ca_cert`, `insecure_skip_verify`, and `org_id`.
  - The endpoint is removed with a `DELETE` request to `/loki-endpoint`.
  - The URL must have a scheme and a host, such as `https://loki.example.com:3100`.
- *Errors:* The socket answers each with the status named in the entry and this text in the body:
  - **`invalid loki endpoint: empty loki endpoint`** (400): *Trigger:* The request names no URL. *Remediation:* Send the URL of a Loki push API.
  - **`invalid loki endpoint: loki endpoint "<url>": <reason>`** (400): *Trigger:* The URL cannot be parsed. *Remediation:* Send a well-formed URL.
  - **`invalid loki endpoint: loki endpoint "<url>" missing scheme or host`** (400): *Trigger:* The URL has no scheme or no host. *Remediation:* Send a full URL.
  - **`request Content-Type must be application/json`** (415): *Trigger:* The request has no `Content-Type: application/json` header. *Remediation:* Send the header.

### Reading the telemetry configuration

Reading the telemetry configuration returns the current tracing and log forwarding values to the agents of the models. An agent reads the workload tracing configuration with `GetControllerTracingConfig` and the log forwarding configuration with `GetControllerLokiConfig`, each for its own tag. A unit agent also reads the charm tracing configuration when it builds the context of its unit.

The operation's required access and rules are:

- *Required access:* An agent of the controller or of a model on it, for its own tag. No model access level applies.
- *Rules:*
  - The values the agent reads are the current ones: the watchers of {ref}`telemetry change streams <telemetry-change-streams>` tell the agent to read again after a change.
  - An agent that reads the log forwarding configuration when none is set receives the error `loki config not found`, with the code `not found`.
