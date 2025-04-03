(command-juju-show-machine)=
# `juju show-machine`

```
Usage: juju show-machine [options] <machineID> ...

Summary:
Show a machine's status.

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
--color  (= false)
    Force use of ANSI color codes
--format  (= yaml)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--utc  (= false)
    Display time as UTC in RFC3339 format

Details:
Show a specified machine on a model.  Default format is in yaml,
other formats can be specified with the "--format" option.
Available formats are yaml, tabular, and json

Examples:
    juju show-machine 0
    juju show-machine 1 2 3
```