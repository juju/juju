(command-juju-create-storage-pool)=
# `juju create-storage-pool`
> See also: [remove-storage-pool](#remove-storage-pool), [update-storage-pool](#update-storage-pool), [storage-pools](#storage-pools)

## Summary
Create or define a storage pool.

## Usage
```juju create-storage-pool [options] <name> <storage provider> [<key>=<value> [<key>=<value>...]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju create-storage-pool ebsrotary ebs volume-type=standard
    juju create-storage-pool gcepd storage-provisioner=kubernetes.io/gce-pd [storage-mode=RWX|RWO|ROX] parameters.type=pd-standard



## Details

Further reading:

- https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-pool
- https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-provider