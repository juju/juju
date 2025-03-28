(command-juju-wait-for_machine)=
# `juju wait-for_machine`
> See also: [wait-for model](#wait-for model), [wait-for application](#wait-for application), [wait-for unit](#wait-for unit)

## Summary
Wait for a machine to reach a specified state.

## Usage
```juju wait-for machine [options] [<id>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--query` | life=="alive" &amp;&amp; status=="started" | query the goal state |
| `--summary` | true | output a summary of the application query on exit |
| `--timeout` | 10m0s | how long to wait, before timing out |

## Examples

Waits for a machine to be created and started.

    juju wait-for machine 0 --query='life=="alive" && status=="started"'


## Details

The wait-for machine command waits for a machine to reach a goal state.
The goal state can be defined programmatically using the query DSL
(domain specific language). The default query for a machine just waits for the
machine to be created and started.

The wait-for command is an optimized alternative to the status command for 
determining programmatically if a goal state has been reached. The wait-for
command streams delta changes from the underlying database, unlike the status
command which performs a full query of the database.

Multiple expressions can be combined to define a complex goal state.