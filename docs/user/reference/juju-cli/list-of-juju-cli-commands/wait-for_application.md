(command-juju-wait-for_application)=
# `juju wait-for_application`
> See also: [wait-for model](#wait-for model), [wait-for machine](#wait-for machine), [wait-for unit](#wait-for unit)

## Summary
Wait for an application to reach a specified state.

## Usage
```juju wait-for application [options] [<name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--query` | life=="alive" &amp;&amp; status=="active" | query the goal state |
| `--summary` | true | output a summary of the application query on exit |
| `--timeout` | 10m0s | how long to wait, before timing out |

## Examples

Waits for 4 units to be present.

    juju wait-for application ubuntu --query='len(units) == 4'

Waits for all the application units to start with ubuntu and to be created 
and available.

    juju wait-for application ubuntu --query='forEach(units, unit => unit.life=="alive" && unit.status=="available" && startsWith(unit.name, "ubuntu"))'


## Details

The wait-for application command waits for the application to reach a goal
state. The goal state can be defined programmatically using the query DSL
(domain specific language). The default query for an application just waits
for the application to be created and active.

The wait-for command is an optimized alternative to the status command for 
determining programmatically if a goal state has been reached. The wait-for
command streams delta changes from the underlying database, unlike the status
command which performs a full query of the database.

The application query DSL can be used to programmatically define the goal state
for machines and units within the scope of the application. This can
be achieved by using lambda expressions to iterate over the machines and units
associated with the application. Multiple expressions can be combined to define 
a complex goal state.