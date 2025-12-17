(command-juju-update-storage-pool)=
# `juju update-storage-pool`
> See also: [create-storage-pool](#create-storage-pool), [remove-storage-pool](#remove-storage-pool), [storage-pools](#storage-pools)

## Summary
Updates storage pool attributes.

## Usage
```juju update-storage-pool [options] <name> [<key>=<value> [<key>=<value>...]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

Update the storage-pool named `iops` with new configuration details:

      juju update-storage-pool operator-storage volume-type=provisioned-iops iops=40

Update which provider the pool is for:

      juju update-storage-pool lxd-storage type=lxd-zfs


## Details

Updates configuration attributes for a single existing storage pool.