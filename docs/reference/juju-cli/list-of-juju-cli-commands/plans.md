(command-juju-plans)=
# `juju plans`

```
Usage: juju plans [options] <charm-url>

Summary:
List plans.

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
-c, --controller (= "")
    Controller to operate in
--format  (= tabular)
    Specify output format (json|smart|summary|tabular|yaml)
-o, --output (= "")
    Specify an output file

Details:
List plans available for the specified charm.

Examples:
    juju plans cs:webapp

Aliases: list-plans
```