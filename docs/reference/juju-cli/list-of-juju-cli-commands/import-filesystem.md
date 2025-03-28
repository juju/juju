(command-juju-import-filesystem)=
# `juju import-filesystem`

```
Usage: juju import-filesystem [options]
<storage-provider> <provider-id> <storage-name>


Summary:
Imports a filesystem into the model.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
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

Examples:
    # Import an existing filesystem backed by an EBS volume,
    # and assign it the "pgdata" storage name. Juju will
    # associate a storage instance ID like "pgdata/0" with
    # the volume and filesystem contained within.
    juju import-filesystem ebs vol-123456 pgdata
```