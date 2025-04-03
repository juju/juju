(command-juju-show-controller)=
# `juju show-controller`

```
Usage: juju show-controller [options] [<controller name> ...]

Summary:
Shows detailed information of a controller.

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
-o, --output (= "")
    Specify an output file
--show-password  (= false)
    Show password for logged in user

Details:
Shows extended information about a controller(s) as well as related models
and user login details.

Examples:
    juju show-controller
    juju show-controller aws google

See also:
    controllers
```