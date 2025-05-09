(command-juju-wait-for)=
# `juju wait-for`
## Summary
Wait for an entity to reach a specified state.

## Usage
```juju wait-for [flags] <command> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--description` | false | Show short description of plugin, if any |
| `-h`, `--help` | false | Show help on a command or other topic. |

## Examples

Waits for the mysql/0 unit to be created and active.

    juju wait-for unit mysql/0

Waits for the mysql application to be active or idle.

    juju wait-for application mysql --query='name=="mysql" && (status=="active" || status=="idle")'

Waits for the model units to all start with ubuntu.

    juju wait-for model default --query='forEach(units, unit => startsWith(unit.name, "ubuntu"))'


## Details
The wait-for set of commands (model, application, machine and unit) defines 
a way to wait for a goal state to be reached. The goal state can be defined
programmatically using the query DSL (domain specific language).

The wait-for command is an optimized alternative to the status command for 
determining programmatically if a goal state has been reached. The wait-for
command streams delta changes from the underlying database, unlike the status
command which performs a full query of the database.

The query DSL is a simple language that can be comprised of expressions to
produce a boolean result. The result of the query is used to determine if the
goal state has been reached. The query DSL is evaluated against the scope of
the command.

Built-in functions are provided to help define the goal state. The built-in
functions are defined in the query package. Examples of built-in functions
include len, print, forEach (lambda), startsWith and endsWith.

See also:
    wait-for model
    wait-for application
    wait-for machine
    wait-for unit

## Subcommands
- [application](#command-juju-wait-for_application)
- [machine](#command-juju-wait-for_machine)
- [model](#command-juju-wait-for_model)
- [unit](#command-juju-wait-for_unit)
