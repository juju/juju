(command-juju-create-storage-pool)=
# `juju create-storage-pool`

```
Usage: juju create-storage-pool [options] <name> <provider> [<key>=<value> [<key>=<value>...]]

Summary:
Create or define a storage pool.

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
Pools are a mechanism for administrators to define sources of storage that
they will use to satisfy application storage requirements.

A single pool might be used for storage from units of many different applications -
it is a resource from which different stores may be drawn.

A pool describes provider-specific parameters for creating storage,
such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD),
or durability.

For many providers, there will be a shared resource
where storage can be requested (e.g. EBS in amazon).
Creating pools there maps provider specific settings
into named resources that can be used during deployment.

Pools defined at the model level are easily reused across applications.
Pool creation requires a pool name, the provider type and attributes for
configuration as space-separated pairs, e.g. tags, size, path, etc.

For Kubernetes models, the provider type defaults to "kubernetes"
unless otherwise specified.

Examples:

    juju create-storage-pool ebsrotary ebs volume-type=standard
    juju create-storage-pool gcepd storage-provisioner=kubernetes.io/gce-pd [storage-mode=RWX|RWO|ROX] parameters.type=pd-standard

See also:
    remove-storage-pool
    update-storage-pool
    storage-pools
```