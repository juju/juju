(command-juju-import-filesystem)=
# `juju import-filesystem`
> See also: [storage](#storage)

## Summary
Imports a filesystem into the model.

## Usage
```juju import-filesystem [options] 
<storage-provider> <provider-id> <storage-name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Import an existing filesystem backed by an EBS volume,
and assign it the "pgdata" storage name. Juju will
associate a storage instance ID like "pgdata/0" with
the volume and filesystem contained within.

    juju import-filesystem ebs vol-123456 pgdata



## Details

Import an existing filesystem into the model. This will lead to the model
taking ownership of the storage, so you must take care not to import storage
that is in use by another Juju model.

To import a filesystem, you must specify three things:

 - the storage provider which manages the storage, and with
   which the storage will be associated
 - the storage provider ID for the filesystem, or
   volume that backs the filesystem
 - the storage name to assign to the filesystem,
   corresponding to the storage name used by a charm

Once a filesystem is imported, Juju will create an associated storage
instance using the given storage name.