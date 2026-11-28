(command-juju-update-storage-pool)=
# `juju update-storage-pool`
> See also: [create-storage-pool](#command-juju-create-storage-pool), [remove-storage-pool](#command-juju-remove-storage-pool), [storage-pools](#command-juju-storage-pools)

## Summary
Replace a storage pool's attributes or change its provider.

## Usage
```text
juju update-storage-pool [options] <name> <key>=<value> [<key>=<value> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Replace the attributes of the EBS pool named `workload-storage`:

      juju update-storage-pool workload-storage volume-type=provisioned-iops iops=40

Change the storage provider of the pool named `workload-storage` to EBS on an AWS model and
set its attributes:

      juju update-storage-pool workload-storage type=ebs volume-type=gp3


## Details

Give the pool name and at least one `key=value` argument. Include every
attribute you want to keep. To check the current attributes before updating,
use `juju storage-pools --name <name>`.

Use `type=<provider>` to select a provider. If that is the only argument
after the pool name, the command sends an empty set of attributes, not the
pool's current attributes.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage-pool/