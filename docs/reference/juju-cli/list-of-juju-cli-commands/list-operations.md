(command-juju-list-operations)=
# `juju list-operations`
> See also: [run](#run), [show-operation](#show-operation), [show-task](#show-task)

**Aliases:** list-operations

## Summary
Lists pending, running, or completed operations for specified application, units, machines, or all.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--actions` |  | Comma separated list of actions names to filter on |
| `--apps`, `--applications` |  | Comma separated list of applications to filter on |
| `--format` | plain | Specify output format (json&#x7c;plain&#x7c;yaml) |
| `--limit` | 0 | The maximum number of operations to return |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--machines` |  | Comma separated list of machines to filter on |
| `-o`, `--output` |  | Specify an output file |
| `--offset` | 0 | Return operations from offset onwards |
| `--status` |  | Comma separated list of operation status values to filter on |
| `--units` |  | Comma separated list of units to filter on |
| `--utc` | false | Show times in UTC |

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

List the operations with the specified query criteria.
When an application is specified, all units from that application are relevant.

When run without any arguments, operations corresponding to actions for all
application units are returned.
To see operations corresponding to juju run tasks, specify an action name
"juju-exec" and/or one or more machines.