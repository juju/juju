(command-juju-whoami)=
# `juju whoami`

```
Usage: juju whoami [options]

Summary:
Print current login details.

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

Details:
Display the current controller, model and logged in user name.

Examples:
    juju whoami

See also:
    controllers
    login
    logout
    models
    users
```