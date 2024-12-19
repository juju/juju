(command-juju-wait-for_model)=
# `juju wait-for_model`
> See also: [wait-for application](#wait-for application), [wait-for machine](#wait-for machine), [wait-for unit](#wait-for unit)

## Summary
Wait for a model to reach a specified state.

## Usage
```juju wait-for model [options] [<name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--query` | life=="alive" &amp;&amp; status=="available" | query the goal state |
| `--summary` | true | output a summary of the application query on exit |
| `--timeout` | 10m0s | how long to wait, before timing out |

## Examples

Waits for all the model units to start with ubuntu.

    juju wait-for model default --query='forEach(units, unit => startsWith(unit.name, "ubuntu"))'

Waits for all the model applications to be active.

    juju wait-for model default --query='forEach(applications, app => app.status == "active")'

Waits for the model to be created and available and for all the model
applications to be active.

    juju wait-for model default --query='life=="alive" && status=="available" && forEach(applications, app => app.status == "active")'


## Details

The wait-for model command waits for the model to reach a goal state. The goal
state can be defined programmatically using the query DSL (domain specific
language). The default query for a model just waits for the model to be
created and available.

The wait-for command is an optimized alternative to the status command for 
determining programmatically if a goal state has been reached. The wait-for
command streams delta changes from the underlying database, unlike the status
command which performs a full query of the database.

The model query DSL can be used to programmatically define the goal state
for applications, machines and units within the scope of the model. This can
be achieved by using lambda expressions to iterate over the applications,
machines and units within the model. Multiple expressions can be combined to 
define a complex goal state.