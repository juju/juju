(command-juju-show-storage)=
# `juju show-storage`

```
Usage: juju show-storage [options] <storage ID> [...]

Summary:
Shows storage instance information.

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
--format  (= yaml)
    Specify output format (json|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
Show extended information about storage instances.
Storage instances to display are specified by storage IDs.
Storage IDs are positional arguments to the command and do not need to be comma
separated when more than one ID is desired.
```