(command-juju-update-storage-pool)=
# `juju update-storage-pool`
> See also: [create-storage-pool](#command-juju-create-storage-pool), [remove-storage-pool](#command-juju-remove-storage-pool), [storage-pools](#command-juju-storage-pools)

## Summary
Update storage pool attributes.

## Usage
```text
juju update-storage-pool [options] <name> [<key>=<value> [<key>=<value>...]]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Update the storage-pool named `iops` with new configuration details:

      juju update-storage-pool workload-storage volume-type=provisioned-iops iops=40

Update which provider the pool is for:

      juju update-storage-pool lxd-storage type=lxd-zfs


## Details

Update configuration attributes for a single existing storage pool.