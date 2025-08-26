(command-juju-remove-storage)=
# `juju remove-storage`
> See also: [add-storage](#add-storage), [attach-storage](#attach-storage), [detach-storage](#detach-storage), [list-storage](#list-storage), [show-storage](#show-storage), [storage](#storage)

## Summary
Removes storage from the model.

## Usage
```juju remove-storage [options] <storage> [<storage> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Remove storage even if it is currently attached |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-destroy` | false | Remove the storage without destroying it |

## Examples

Remove the detached storage `pgdata/0`:

    juju remove-storage pgdata/0

Remove the possibly attached storage `pgdata/0`:

    juju remove-storage --force pgdata/0

Remove the storage `pgdata/0`, without destroying
the corresponding cloud storage:

    juju remove-storage --no-destroy pgdata/0



## Details

Removes storage from the model. Specify one or more
storage IDs, as output by `juju storage`.

By default, `remove-storage` will fail if the storage
is attached to any units. To override this behaviour,
you can use `juju remove-storage --force`.
Note: Forced detach is not available on container models.