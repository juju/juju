(command-juju-detach-storage)=
# `juju detach-storage`

```
Usage: juju detach-storage [options] <storage> [<storage> ...]

Summary:
Detaches storage from units.

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
    Forcefully detach storage
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
Detaches storage from units. Specify one or more unit/application storage IDs,
as output by "juju storage". The storage will remain in the model until it is
removed by an operator.

Detaching storage may fail but under some circumstances, Juju user may need
to force storage detachment despite operational errors.


Examples:
    juju detach-storage pgdata/0
    juju detach-storage --force pgdata/0
```