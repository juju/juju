---
myst:
  html_meta:
    description: "Juju telemetry reference: the daily business metrics that models send to Charmhub, and the OpenTelemetry tracing that the controller, agents and charms can export to a collector."
---

(telemetry)=
# Telemetry

Telemetry is the automatic recording and transmission of data from remote sources. In Juju, it covers two independent mechanisms: {ref}`business metrics <telemetry-business-metrics>` that help the developers improve Juju, and {ref}`OpenTelemetry tracing <telemetry-tracing>` that helps you observe your own deployment.

(telemetry-business-metrics)=
## Business metrics

Each model gathers routine business metrics and sends them to [Charmhub](https://charmhub.io/) together with its daily check for new charm revisions. The metrics help the developers of Juju improve it.

```{note}
No user information is gathered.
```

### What data is collected?

* For a controller
  * juju version
  * controller uuid
* For a model
  * number of applications
  * number of units deployed
  * number of machines deployed
  * cloud
  * cloud provider
  * cloud region
  * model uuid
* For each application
  * number of units

### What is the data used for?

The data helps the developers gain a better understanding of how Juju is used in the field:

* Which clouds are the most popular?
* How many applications, units and machines do models typically have?
* How many models are used with a controller?

For example, the developers can better design improvements.

### How do I disable data collection?

To disable business metrics in a Juju model, set the {ref}`disable-telemetry <model-config-disable-telemetry>` model configuration key to `true`:

```text
juju model-config disable-telemetry=true
```

The daily check for new charm revisions continues to run and omits the metrics.

(telemetry-tracing)=
## OpenTelemetry tracing

The controller and the agents in your models can export [OpenTelemetry](https://opentelemetry.io/) traces to a collector that you operate. Charms can write spans to the same collector.

Tracing stays off until an OTLP collector endpoint is set.

### Workload tracing

Workload tracing covers the spans that Juju itself exports. The controller stores the configuration and distributes it to the controller and to the agents, so a single setting governs the whole system. Setting an HTTP or a gRPC endpoint turns tracing on; clearing both turns it off.

The configuration takes these fields:

* `http_endpoint` and `grpc_endpoint`: the OTLP endpoints of the collector.
* `ca_cert`: the CA certificate used to verify the collector.
* `insecure_skip_verify`: whether to skip verifying the collector's certificate.
* `stack_traces`: whether to attach stack traces to spans.
* `sample_ratio`: the fraction of spans to sample, between `0` and `1` (default: `0.1`).
* `tail_sampling_threshold`: a non-negative duration, for example `1ms` (default: `1ms`).

A field that is omitted or empty is removed from the configuration.

### Charm tracing

Charm tracing gives the charms in your models a collector to write spans to. The configuration takes the fields `http_endpoint`, `grpc_endpoint` and `ca_cert`. When any of them is set, the unit agent passes them to the charm in the environment variables `JUJU_CHARM_TRACE_CONFIG_HTTP`, `JUJU_CHARM_TRACE_CONFIG_GRPC` and `JUJU_CHARM_TRACE_CONFIG_CA_CERT`, so the charm needs no integration with the collector.

### How is tracing configured?

The `juju-controller` charm sets both configurations through the controller's control socket, a Unix socket named `control.socket` in the data directory of the controller machine agent. The socket accepts a JSON `POST` request at `/workload-tracing-config` and at `/charm-tracing-config`.

## Relationship to previous releases

Earlier Juju releases also collected the names of the charms that an application is related to, and the `transmit-vendor-metrics` model configuration key controlled the upload of the metrics that charms declared. Juju 4.1 sends neither: It has dropped the relation names, and the `transmit-vendor-metrics` key has no effect.
