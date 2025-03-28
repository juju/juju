(command-juju-update-storage-pool)=
# `juju update-storage-pool`

```
Usage: juju update-storage-pool [options] <name> [<key>=<value> [<key>=<value>...]]

Summary:
Update storage pool attributes.

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
Update configuration attributes for a single existing storage pool.

Example:
    # Update the storage-pool named iops with new configuration details

      juju update-storage-pool operator-storage volume-type=provisioned-iops iops=40

    # Update which provider the pool is for
      juju update-storage-pool lxd-storage type=lxd-zfs

See also:
    create-storage-pool
    remove-storage-pool
    storage-pools
```