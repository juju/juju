(juju-exec)=
# `juju-exec`

## Summary

Run commands in a unit's hook context.

## Usage

```text
juju-exec [options] [-u] [<unit-name>] <commands>
```

### Options


```text
--force-remote-unit  (= false)
    run the commands for a specific relation context, bypassing the remote unit check
--no-context  (= false)
    do not run the command in a unit context
--operator  (= false)
    run the commands on the operator instead of the workload. Only supported on k8s workload charms
-r, --relation (= "")
    run the commands for a specific relation context on a unit
--remote-app (= "")
    run the commands for a specific remote application in a relation context on a unit
--remote-unit (= "")
    run the commands for a specific remote unit in a relation context on a unit
-u (= "-")
    explicit unit-name, all other arguments are commands. if -u is passed an empty string, unit-name is inferred from state

Details:
Run the specified commands in the hook context for the unit.

unit-name can be either the unit tag:
 i.e.  unit-ubuntu-0
or the unit id:
 i.e.  ubuntu/0

unit-name can be specified by the -u argument.
If -u is passed, unit-name cannot be passed as a positional argument.

If --no-context is specified, the <unit-name> positional
argument or -u argument is not needed.

If the there's one and only one unit on this host, <unit-name>
is automatically inferred and the positional argument is not needed.
If -u is passed an empty string, this behaviour is also observed.


## Examples


	juju-exec app/0 hostname -f
	juju-exec --no-context -- hostname -f
	juju-exec "hostname -f"
	juju-exec -u "" -- hostname -f
	juju-exec -u app/0 "hostname -f"
	juju-exec -u app/0 -- hostname -f

The commands are executed with '/bin/bash -s', and the output returned.
