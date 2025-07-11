(juju_metrics)=
# `juju_metrics`


The Juju metrics introspection tool provides the current values of metrics which Juju tracking. Some of these are more interesting over time using a tool such as grafana.

This is primarily useful to developers to help debug problems that may be occurring in deployed systems. Advance admins can use the data to see when investigation is required, or an error is hidden.

## Usage

Can be run on any juju machine.

```text
juju_metrics
```

## Example output

```text
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.6921e-05
go_gc_duration_seconds{quantile="0.25"} 8.3083e-05
go_gc_duration_seconds{quantile="0.5"} 9.8263e-05
go_gc_duration_seconds{quantile="0.75"} 0.00013904
go_gc_duration_seconds{quantile="1"} 0.000921937
go_gc_duration_seconds_sum 0.201048689
go_gc_duration_seconds_count 1521
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 704
# HELP go_info Information about the Go environment.
# TYPE go_info gauge
go_info{version="go1.18.1"} 1
# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 2.3154496e+07
# and many more
```

## Interesting Output

* `process_open_fds`: how many file descriptors are open. This should not grow over time in a stable Juju deployment.

* `juju_dependency_engine_worker_start`: how many times a dependency has started. Each dependency has an individual number. None should have a number significantly higher than the rest. This indicates the worker is restarting due to an error. Also seen in the {ref}`juju_engine_report`.

* `go_goroutines`: a gauge for the current number of goroutines. Should not be growing in a stable config.
