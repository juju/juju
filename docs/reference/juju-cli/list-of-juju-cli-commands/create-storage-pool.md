(command-juju-create-storage-pool)=
# `juju create-storage-pool`
> See also: [remove-storage-pool](#remove-storage-pool), [update-storage-pool](#update-storage-pool), [storage-pools](#storage-pools)

## Summary
Creates or defines a storage pool.

## Usage
```juju create-storage-pool [options] <name> <storage provider> [<key>=<value> [<key>=<value>...]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju create-storage-pool ebsrotary ebs volume-type=standard
    juju create-storage-pool gcepd storage-provisioner=kubernetes.io/gce-pd [storage-mode=RWX|RWO|ROX] parameters.type=pd-standard



## Details

Further reading:

- https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-pool
- https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-provider