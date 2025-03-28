(command-juju-enable-destroy-controller)=
# `juju enable-destroy-controller`

```
Usage: juju enable-destroy-controller [options]

Summary:
Enable destroy-controller by removing disabled commands in the controller.

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
-c, --controller (= "")
    Controller to operate in

Details:
Any model in the controller that has disabled commands will block a controller
from being destroyed.

A controller administrator is able to enable all the commands across all the models
in a Juju controller so that the controller can be destoyed if desired.

See also:
    disable-command
    disabled-commands
    enable-command
```