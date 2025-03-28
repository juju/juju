(command-juju-unregister)=
# `juju unregister`

```
Usage: juju unregister [options] <controller name>

Summary:
Unregisters a Juju controller.

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
-y, --yes, --no-prompt  (= false)
    Do not prompt for confirmation

Details:
Removes local connection information for the specified controller.  This
command does not destroy the controller.  In order to regain access to an
unregistered controller, it will need to be added again using the juju register
command.

Examples:

    juju unregister my-controller

See also:
    destroy-controller
    kill-controller
    register
```