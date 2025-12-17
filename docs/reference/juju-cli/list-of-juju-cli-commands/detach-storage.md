(command-juju-detach-storage)=
# `juju detach-storage`
> See also: [storage](#storage), [attach-storage](#attach-storage)

## Summary
Detaches storage from units.

## Usage
```juju detach-storage [options] <storage> [<storage> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--force` | false | Specifies whether to forcibly detach storage. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju detach-storage pgdata/0
    juju detach-storage --force pgdata/0



## Details

Detaches storage from units. Specify one or more unit/application storage IDs,
as output by `juju storage`. The storage will remain in the model until it is
removed by an operator.

Detaching storage may fail but under some circumstances, Juju user may need
to force storage detachment despite operational errors.