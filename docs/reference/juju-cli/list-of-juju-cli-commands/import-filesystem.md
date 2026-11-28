(command-juju-import-filesystem)=
# `juju import-filesystem`
> See also: [storage](#command-juju-storage)

## Summary
Imports a filesystem into the model.

## Usage
```text
juju import-filesystem [options] <pool> <provider-id> <storage-name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Request provider-specific forced import; ownership checks still apply |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Import an unused EBS volume containing a filesystem for the pgdata storage name:

    juju import-filesystem ebs vol-123456 pgdata

Import the unbound Kubernetes PersistentVolume pv-data-001 after checking
its reclaim policy is Retain and that no other workload uses it:

    juju import-filesystem kubernetes pv-data-001 pgdata


## Details

Adopt an existing filesystem into this model. Supply the name of an existing
storage pool, the filesystem's provider ID (or its backing volume's provider
ID), and a filesystem storage name accepted by the intended charm. Import
reports a new, detached Juju instance ID. It does not copy data or attach it.

Check that the filesystem is free for reuse. After a failed import, inspect
the provider object before retrying. On Kubernetes, check the volume and its claim.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/