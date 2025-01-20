(juju_agent)=
# `juju_agent`

The juju_agent script is a proxy function which 
will pass the arguments to the correct agent (
machine, controller, application or unit).
It is used to simplify the usage of the juju-introspect.

## Usage

```text
juju_agent [command]                                  
```

## Details

juju_agent is one of the shell functions that 
is exported by the juju-introspect shell script. 
This subcommand is simply a selector for the  
correct agent to use when passing another subcommand
to juju-instrospect.

By default, juju_agent will transfer the subcommand  
to the machine agent. 
If the machine agent is not running (i.e. if the 
agent name is null), then it tries to transfer 
to the controller agent. If the controller agent is 
not running, then it tries to transfer to the application 
agent. And, finally, if the application agent is not 
running, then it tries to transfer to the unit agent.

## Example 
```code
juju_agent metrics
```
_Note: This example is equivalent to running juju_metrics._


Output:
```text
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.3922e-05
go_gc_duration_seconds{quantile="0.25"} 7.653e-05
go_gc_duration_seconds{quantile="0.5"} 0.000108833
go_gc_duration_seconds{quantile="0.75"} 0.000218571
go_gc_duration_seconds{quantile="1"} 0.001983067
go_gc_duration_seconds_sum 0.006144371
go_gc_duration_seconds_count 27
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 1974
...
```

