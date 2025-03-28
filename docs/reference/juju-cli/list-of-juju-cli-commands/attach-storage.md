(command-juju-attach-storage)=
# `juju attach-storage`

```
Usage: juju attach-storage [options] <unit> <storage> [<storage> ...]

Summary:
Attaches existing storage to a unit.

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
Attach existing storage to a unit. Specify a unit
and one or more storage IDs to attach to it.

Examples:
    juju attach-storage postgresql/1 pgdata/0
```