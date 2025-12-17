(command-juju-remove-storage-pool)=
# `juju remove-storage-pool`
> See also: [create-storage-pool](#create-storage-pool), [update-storage-pool](#update-storage-pool), [storage-pools](#storage-pools)

## Summary
Removes an existing storage pool.

## Usage
```juju remove-storage-pool [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

Remove the storage-pool named fast-storage:

      juju remove-storage-pool fast-storage


## Details

Removes a single existing storage pool.