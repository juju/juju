(command-juju-operations)=
# `juju operations`
> See also: [run](#run), [show-operation](#show-operation), [show-task](#show-task)

**Aliases:** list-operations

## Summary
Lists pending, running, or completed operations for specified application, units, machines, or all.

## Usage
```juju operations [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--actions` |  | Specifies a comma-separated list of action names to filter on. |
| `--apps`, `--applications` |  | Specifies a comma-separated list of applications to filter on. |
| `--format` | plain | Specify output format (json&#x7c;plain&#x7c;yaml) |
| `--limit` | 0 | Specifies the maximum number of operations to return. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--machines` |  | Specifies a comma-separated list of machines to filter on. |
| `-o`, `--output` |  | Specify an output file |
| `--offset` | 0 | Returns operations from offset onwards. |
| `--status` |  | Specifies a comma-separated list of operation status values to filter on. |
| `--units` |  | Specifies a comma-separated list of units to filter on. |
| `--utc` | false | Specifies whether to show times in UTC. |

## Examples

    juju operations
    juju operations --format yaml
    juju operations --actions juju-exec
    juju operations --actions backup,restore
    juju operations --apps mysql,mediawiki
    juju operations --units mysql/0,mediawiki/1
    juju operations --machines 0,1
    juju operations --status pending,completed
    juju operations --apps mysql --units mediawiki/0 --status running --actions backup



## Details

Lists the operations with the specified query criteria.

When an application is specified, all units from that application are relevant.

When run without any arguments, operations corresponding to actions for all
application units are returned.
To see operations corresponding to `juju run` tasks, specify an action name,
`juju-exec`, and/or one or more machines.