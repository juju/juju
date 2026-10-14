# Juju metrics

This document describes the Prometheus metrics emitted by Juju controller
agents. It is versioned with Juju because metric names, types, meanings, and
labels are part of the controller's operational API.

## Accessing metrics

A controller agent exposes its Prometheus registry at the authenticated HTTPS
path `/metrics/`. The endpoint uses the controller API listener, so its host
and port are the controller API address advertised by `juju controllers`.
It is not a public unauthenticated endpoint.

For normal operation, relate the controller application's `metrics-endpoint`
to Prometheus. The `juju-controller` charm supplies the endpoint address and
the credentials and CA required to scrape it. See [Collect metrics about a
controller](docs/howto/manage-controllers.md#collect-metrics-about-a-controller)
for an example using the Canonical Observability Stack.

The same registry is available on Linux agents through their local
introspection socket at `/metrics/`. That interface is intended for local
diagnostics, not remote scraping.

The endpoint emits Prometheus text exposition format. A metric family appears
only after its collector has been registered, and a labelled series normally
appears only after it has been observed. Consequently, a scrape of an idle
controller is not a complete inventory.

## Labels and compatibility

`namespace` identifies a controller or model database namespace. `model_uuid`
identifies a model. These labels have a bounded set per controller, but their
values can change as models are created and removed. `host` is the remote host
used by an outbound HTTP request. `endpoint`, `method`, `status`, `result`,
`error`, `reason`, `auth_method`, `failed`, and `model_type` use bounded values
chosen by Juju or the remote protocol.

Do not add labels derived from request paths, entity names, credentials, or
other user-controlled values. In particular, HTTP client metrics deliberately
use the remote host rather than the request path.

Metric names and label sets are compatibility contracts. A breaking change
must use a new metric name and retain the old one for a documented deprecation
period. Most counters use the Prometheus `_total` suffix; histograms also
expose `_bucket`, `_sum`, and `_count` series, and summaries expose `_sum` and
`_count` series.

## Controller API server

| Metric | Type | Labels | Unit | Description |
| --- | --- | --- | --- | --- |
| `juju_apiserver_build_info` | gauge | `version`, `official_build`, `go_compiler`, `go_version`, `git_commit`, `git_commit_state`, `go_build_tags` | 1 | Build identity for the controller process. |
| `juju_apiserver_connections_total` | counter | none | connections | API-server connections accepted over the process lifetime. |
| `juju_apiserver_connections` | gauge | `endpoint` | connections | Currently active API-server connections by API handler. |
| `juju_apiserver_active_login_attempts` | gauge | none | attempts | Agent logins currently in progress. |
| `juju_apiserver_request_duration_seconds` | summary | `facade`, `version`, `method`, `error_code` | seconds | API request latency. `error_code` is empty on success and `error` on failure. |
| `juju_apiserver_ping_failure_count` | counter | `model_uuid`, `endpoint` | failures | Agent ping failures. |
| `juju_apiserver_log_write_count` | counter | `model_uuid`, `state` | writes | Log writes handled by the API server. |
| `juju_apiserver_log_read_count` | counter | `model_uuid`, `state` | reads | Log reads handled by the API server. |
| `juju_apiserver_outbound_requests_total` | counter | `model_uuid`, `host`, `status` | requests | Outbound API HTTP requests. |
| `juju_apiserver_outbound_request_errors_total` | counter | `model_uuid`, `host` | requests | Failed outbound API HTTP requests. |
| `juju_apiserver_outbound_request_duration_seconds` | summary | `model_uuid`, `host` | seconds | Latency of outbound API HTTP requests. |
| `juju_apiserver_ssh_relay_session_duration_seconds` | histogram | none | seconds | Time relayed SSH sessions remain open. |
| `juju_apiserver_ssh_relay_rejections_total` | counter | `reason` | requests | Rejected SSH relay requests. |

## Database and change streams

| Metric | Type | Labels | Unit | Description |
| --- | --- | --- | --- | --- |
| `juju_db_requests_total` | gauge | `namespace` | requests | Active database requests. Despite its name, this is a gauge. |
| `juju_db_duration_seconds` | histogram | `namespace`, `result` | seconds | Time spent handling database requests. |
| `juju_db_errors_total` | counter | `namespace`, `error` | errors | Database errors by transaction error class. |
| `juju_db_success_total` | counter | `namespace` | operations | Successful database operations. |
| `juju_db_txn_requests_total` | counter | `namespace` | requests | Transaction requests, including retries. |
| `juju_db_txn_retries_total` | counter | `namespace` | retries | Transaction retries. |
| `juju_db_watermark_inserts_total` | counter | `namespace` | insertions | Changelog witness-table watermark insertions. |
| `juju_db_watermark_retries_total` | counter | `namespace` | retries | Retries while inserting changelog watermarks. |
| `juju_db_changestream_duration_seconds` | histogram | `namespace` | seconds | Time spent serving change-stream requests. |
| `juju_db_changestream_count` | histogram | `namespace` | changes | Changes returned by a change-stream request. |
| `juju_db_subscription_count` | gauge | `namespace` | subscriptions | Current event-multiplexer subscriptions. |
| `juju_db_dispatch_duration_seconds` | histogram | `namespace`, `failed` | seconds | Event-multiplexer dispatch duration. |

## Agent workers

| Metric | Type | Labels | Unit | Description |
| --- | --- | --- | --- | --- |
| `juju_dependency_engine_worker_start` | gauge | `worker` | starts | Worker starts recorded by the dependency engine. It is a gauge that is incremented for each start. |
| `juju_dependency_engine_worker_starts_for_model` | gauge | `model_uuid` | starts | Worker starts recorded for a model. The series is removed when the model worker is removed. |
| `juju_http_client_worker_outbound_requests_total` | counter | `host`, `status` | requests | Outbound HTTP requests made by the shared HTTP client worker. |
| `juju_http_client_worker_outbound_request_errors_total` | counter | `host`, `status` | requests | Failed outbound HTTP requests made by the shared HTTP client worker. |
| `juju_http_client_worker_outbound_request_duration_seconds` | summary | `host` | seconds | Outbound HTTP request latency. |
| `juju_control_socket_requests_total` | counter | `endpoint`, `method`, `status` | requests | Control-socket requests. |
| `juju_control_socket_request_errors_total` | counter | `endpoint`, `method`, `status` | requests | Failed control-socket requests. |
| `juju_control_socket_request_duration_seconds` | histogram | `endpoint`, `method` | seconds | Control-socket request latency. |
| `juju_sshserver_connection_count` | gauge | none | connections | Active connections to the controller SSH server. |
| `juju_sshserver_connection_duration_seconds` | histogram | none | seconds | Time an SSH server connection remains open. |
| `juju_sshserver_time_to_session_seconds` | histogram | `model_type` | seconds | Time required to establish an SSH session. |
| `juju_sshserver_authentication_failures` | counter | `auth_method` | failures | Rejected SSH authentication attempts. |

## Optional log forwarding

The following metrics are registered only when the Loki log-forwarding backend
is configured and running.

| Metric | Type | Labels | Unit | Description |
| --- | --- | --- | --- | --- |
| `juju_loki_forwarder_sent_total` | counter | `log_router` | log records | Log records successfully sent to Loki. |
| `juju_loki_forwarder_dropped_total` | counter | `log_router` | log records | Log records dropped from the local queue. |
| `juju_loki_forwarder_push_errors_total` | counter | `log_router` | attempts | Loki push attempts that failed after retries. |

`log_router` distinguishes the API-backed and controller-local log routers.

## Runtime metrics

Every Linux agent registry also includes the standard collectors supplied by
`prometheus/client_golang`: `go_*` runtime and garbage-collector metrics, and
`process_*` CPU, file-descriptor, memory, start-time, and virtual-memory
metrics. These are third-party collector metrics rather than Juju metric
contracts; their exact families depend on the Go and operating-system versions.
Consult the `HELP` and `TYPE` records returned by the endpoint for their
version-specific definitions.

## Maintaining this document

When adding, removing, or changing a collector, update this file in the same
change. Confirm both an idle scrape and a scrape after exercising the feature:
collectors and labelled series can be conditional. Every Juju-owned metric
must provide a Prometheus `HELP` string, a type, a base unit where applicable,
and a bounded label set.
