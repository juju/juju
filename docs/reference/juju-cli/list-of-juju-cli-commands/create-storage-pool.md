(command-juju-create-storage-pool)=
# `juju create-storage-pool`
> See also: [remove-storage-pool](#command-juju-remove-storage-pool), [update-storage-pool](#command-juju-update-storage-pool), [storage-pools](#command-juju-storage-pools)

## Summary
Create a storage pool in the selected model.

## Usage
```text
juju create-storage-pool [options] <name> [<provider>] [<key>=<value> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Create an EBS pool named `workload-storage` for gp3 volumes on an AWS model:

    juju create-storage-pool workload-storage ebs volume-type=gp3

Create a pool named `fast-storage` using the `fast` storage class on a Kubernetes model:

    juju create-storage-pool fast-storage storage-class=fast storage-mode=RWO


## Details

On a machine model, give the pool name followed by the storage provider.
On a Kubernetes model, omit the provider to use `kubernetes`.
Give each attribute as a separate `key=value` argument. The keys
`name` and `type` are reserved.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage-pool/