(command-juju-actions)=
# `juju actions`

```
Usage: juju actions [options] <application>

Summary:
List actions defined for an application.

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
--format  (= default)
    Specify output format (default|json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--schema  (= false)
    Display the full action schema

Details:
List the actions available to run on the target application, with a short
description.  To show the full schema for the actions, use --schema.

Examples:
    juju list-actions postgresql
    juju list-actions postgresql --format yaml
    juju list-actions postgresql --schema

See also:
    run-action
    show-action

Aliases: list-actions

```