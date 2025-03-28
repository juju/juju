(command-juju-remove-storage-pool)=
# `juju remove-storage-pool`

```
Usage: juju remove-storage-pool [options] <name>

Summary:
Remove an existing storage pool.

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
Remove a single existing storage pool.

Example:
    # Remove the storage-pool named fast-storage

      juju remove-storage-pool fast-storage

See also:
    create-storage-pool
    update-storage-pool
    storage-pools
```