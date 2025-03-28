(command-juju-set-meter-status)=
# `juju set-meter-status`

```
Usage: juju set-meter-status [options] [application or unit] status

Summary:
Sets the meter status on an application or unit.

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
--info (= "")
    Set the meter status info to this string
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
Set meter status on the given application or unit. This command is used
to test the meter-status-changed hook for charms in development.

Examples:
    juju set-meter-status myapp RED
    juju set-meter-status myapp/0 AMBER --info "my message"
```