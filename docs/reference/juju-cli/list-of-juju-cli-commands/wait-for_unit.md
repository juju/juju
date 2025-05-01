(command-juju-wait-for_unit)=
# `juju wait-for_unit`
> See also: [wait-for model](#command-juju-wait-for_model), [wait-for application](#command-juju-wait-for_application), [wait-for machine](#command-juju-wait-for_machine)

## Summary
Wait for a unit to reach a specified state.

## Usage
```juju wait-for unit [options] [<name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--query` | life=="alive" &amp;&amp; workload-status=="active" | query the goal state |
| `--summary` | true | output a summary of the application query on exit |
| `--timeout` | 10m0s | how long to wait, before timing out |

## Examples

Waits for a units to be machines to be length of 1.

    juju wait-for unit ubuntu/0 --query='len(machines) == 1'

Waits for the unit to be created and active.

    juju wait-for unit ubuntu/0 --query='life=="alive" && workload-status=="active"'


## Details

The wait-for unit command waits for the unit to reach a goal state. The goal
state can be defined programmatically using the query DSL (domain specific
language). The default query for a unit just waits for the unit to be created 
and active.

The wait-for command is an optimized alternative to the status command for 
determining programmatically if a goal state has been reached. The wait-for
command streams delta changes from the underlying database, unlike the status
command which performs a full query of the database.

The unit query DSL can be used to programmatically define the goal state
for machine within the scope of the unit. This can be achieved by using lambda
expressions to iterate over the machines associated with the unit. Multiple
expressions can be combined to define a complex goal state.