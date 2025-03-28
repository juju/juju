(command-juju-set-plan)=
# `juju set-plan`

```
Usage: juju set-plan [options] <application name> <plan>

Summary:
Set the plan for an application.

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
Set the plan for the deployed application, effective immediately.

The specified plan name must be a valid plan that is offered for this
particular charm. Use "juju list-plans <charm>" for more information.

Examples:
    juju set-plan myapp example/uptime
```