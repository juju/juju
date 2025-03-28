(command-juju-show-action)=
# `juju show-action`

```
Usage: juju show-action [options] <application> <action>

Summary:
Shows detailed information about an action.

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
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
Show detailed information about an action on the target application.

Examples:
    juju show-action postgresql backup

See also:
    list-actions
    run-action
```