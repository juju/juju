(command-juju-controllers)=
# `juju controllers`

```
Usage: juju controllers [options]

Summary:
Lists all controllers.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file
--refresh  (= false)
    Connect to each controller to download the latest details

Details:
The output format may be selected with the '--format' option. In the
default tabular output, the current controller is marked with an asterisk.

Examples:
    juju controllers
    juju controllers --format json --output ~/tmp/controllers.json

See also:
    models
    show-controller

Aliases: list-controllers
```