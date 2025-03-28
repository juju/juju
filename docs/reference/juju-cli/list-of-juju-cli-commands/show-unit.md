(command-juju-show-unit)=
# `juju show-unit`

```
Usage: juju show-unit [options] <unit name>

Summary:
Displays information about a unit.

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
--app  (= false)
    only show application relation data
--endpoint (= "")
    only show relation data for the specified endpoint
--format  (= yaml)
    Specify output format (json|smart|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--related-unit (= "")
    only show relation data for the specified unit

Details:
The command takes deployed unit names as an argument.

Optionally, relation data for only a specified endpoint
or related unit may be shown, or just the application data.

Examples:
    juju show-unit mysql/0
    juju show-unit mysql/0 wordpress/1
    juju show-unit mysql/0 --app
    juju show-unit mysql/0 --endpoint db
    juju show-unit mysql/0 --related-unit wordpress/2
```