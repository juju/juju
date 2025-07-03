(juju_presence_report)=
# `juju_presence_report`

`juju_presence_report` shows the status of Juju agent connections to a controller
and can be used to establish a view on which agents are "alive". A connection
is registered whenever an agent logs in to the API server on the machine on
which the report is generated.

The agents for each model are listed together beneath the UUID of that model.

## Usage
Must be run on a Juju controller machine.
```code
juju_presence_report
```

## Example output
```text
$ juju_presence_report 
[2e15302a-bbdf-4077-8e65-116a5a0e0348]

AGENT              SERVER     CONN ID  STATUS
machine-0          machine-0  4        alive
machine-0          machine-0  19       alive
machine-0          machine-2  2        alive
machine-0          machine-3  1        alive
machine-1          machine-0  17       alive
machine-2          machine-0  42       alive
machine-2          machine-0  47       alive
machine-2          machine-0  48       alive
machine-2          machine-3  2        alive
machine-3          machine-0  31       alive
machine-3          machine-0  44       alive
machine-3          machine-0  45       alive
machine-3          machine-2  1        alive
unit-controller-0  machine-0  21       alive
unit-controller-1  machine-0  43       alive
unit-controller-2  machine-0  36       alive
unit-ubuntu-0      machine-0  18       alive

[73e18e39-035a-4ceb-8cd6-dbba16fd9c16]

AGENT                   SERVER     CONN ID  STATUS
machine-0 (controller)  machine-0  2        alive
machine-2 (controller)  machine-3  3        alive
machine-3 (controller)  machine-0  46       alive
```
