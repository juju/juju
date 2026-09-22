---
myst:
  html_meta:
    description: "Juju telemetry reference: OpenTelemetry tracing exported by the Juju controller and agents to an OTLP collector endpoint, configured via controller configuration keys."
---

(telemetry)=
# Telemetry

Telemetry is the automatic recording and transmission of data from remote sources. In Juju, telemetry means [OpenTelemetry](https://opentelemetry.io/) tracing: the controller and its agents export OpenTelemetry spans for their operations to a collector endpoint that you configure. Tracing is disabled by default and controlled through {ref}`controller configuration <controller-configuration>`.

The controller distributes the endpoint and related settings to the agents as part of the agent configuration, so a single controller-level setting governs the whole system.

## Controller configuration keys

Tracing is configured with the following {ref}`controller configuration <list-of-controller-configuration-keys>` keys:

* `open-telemetry-enabled`: whether tracing is enabled (default: `false`)
* `open-telemetry-endpoint`: the OTLP endpoint the traces are pushed to (for example, a collector's gRPC or HTTP endpoint)
* `open-telemetry-insecure`: whether the collector endpoint is insecure (useful for debug or local testing; default: `false`)
* `open-telemetry-stack-traces`: whether stack traces are attached to spans
* `open-telemetry-sample-ratio`: the sampling ratio for spans (default: `0.10`)
* `open-telemetry-tail-sampling-threshold`: the tail sampling threshold, as a duration

For example, to enable tracing against a local collector:

```text
juju controller-config open-telemetry-enabled=true open-telemetry-endpoint=localhost:4317 open-telemetry-insecure=true
```

```{note}
No user information is gathered.
```

## Relationship to previous releases

Previous Juju releases included a vendor-metrics pipeline: models collected routine business metrics (deployment statistics, charm metrics, cloud usage), which were uploaded once a day for anonymised aggregate analytics. That pipeline has been removed in Juju 4.0. The `disable-telemetry` and `transmit-vendor-metrics` model configuration keys still exist for backwards compatibility but have no effect in Juju 4.0.