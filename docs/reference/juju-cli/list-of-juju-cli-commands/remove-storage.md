(command-juju-remove-storage)=
# `juju remove-storage`

```
Usage: juju remove-storage [options] <storage> [<storage> ...]

Summary:
Removes storage from the model.

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
--force  (= false)
    Remove storage even if it is currently attached
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-destroy  (= false)
    Remove the storage without destroying it

Details:
Removes storage from the model. Specify one or more
storage IDs, as output by "juju storage".

By default, remove-storage will fail if the storage
is attached to any units. To override this behaviour,
you can use "juju remove-storage --force".
Note: forced detach is not available on container models.

Examples:
    # Remove the detached storage pgdata/0.
    juju remove-storage pgdata/0

    # Remove the possibly attached storage pgdata/0.
    juju remove-storage --force pgdata/0

    # Remove the storage pgdata/0, without destroying
    # the corresponding cloud storage.
    juju remove-storage --no-destroy pgdata/0
```