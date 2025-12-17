(command-juju-attach-storage)=
# `juju attach-storage`
## Summary
Attaches existing storage to a unit.

## Usage
```juju attach-storage [options] <unit> <storage> [<storage> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju attach-storage postgresql/1 pgdata/0



## Details

Attach existing storage to a unit. Specify a unit
and one or more storage IDs to attach to it.