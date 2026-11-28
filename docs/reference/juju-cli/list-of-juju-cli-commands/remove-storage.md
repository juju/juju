(command-juju-remove-storage)=
# `juju remove-storage`
> See also: [add-storage](#command-juju-add-storage), [attach-storage](#command-juju-attach-storage), [detach-storage](#command-juju-detach-storage), [show-storage](#command-juju-show-storage), [storage](#command-juju-storage)

## Summary
Removes storage from the model.

## Usage
```text
juju remove-storage [options] <storage> [<storage> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Request detachment before removal (machine models only) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-destroy` | false | Remove the storage without destroying it |

## Examples

Release the detached pgdata/0 filesystem after recording its provider ID:

    juju remove-storage --no-destroy pgdata/0

Remove the detached pgdata/0 instance and request destruction of its backing
after verifying that you no longer need its data:

    juju remove-storage pgdata/0


## Details

Remove a storage instance from the model and request destruction of its
backing. Use `--no-destroy` to release the backing. Both forms remove the
Juju instance ID. Record the provider ID first if you need to find the backing later.

Wait for detachment to finish. On machine models, `--force` requests
detachment before removal. Check the provider inventory to confirm what remains.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/