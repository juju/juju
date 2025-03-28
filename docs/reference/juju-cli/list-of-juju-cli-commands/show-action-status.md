(command-juju-show-action-status)=
# `juju show-action-status`

```
Usage: juju show-action-status [options] [<action>|<action-id-prefix>]

Summary:
Show results of all actions filtered by optional ID prefix.

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
--name (= "")
    Action name
-o, --output (= "")
    Specify an output file

Details:
Show the status of Actions matching given ID, partial ID prefix, or all Actions if no ID is supplied.
If --name <name> is provided the search will be done by name rather than by ID.
```