(command-juju-cancel-action)=
# `juju cached-cancel-action`

```
Usage: juju cancel-action [options] (<action-id>|<action-id-prefix>) [...]

Summary:
Cancel pending or running actions.

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
Cancel pending or running actions matching given IDs or partial ID prefixes.

Aliases: cancel-task
```