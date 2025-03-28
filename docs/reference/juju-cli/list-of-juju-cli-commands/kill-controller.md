(command-juju-kill-controller)=
# `juju kill-controller`

```
Usage: juju kill-controller [options] <controller name>

Summary:
Forcibly terminate all machines and other associated resources for a Juju controller.

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
-t, --timeout  (= 5m0s)
    Timeout before direct destruction
-y, --yes, --no-prompt  (= false)
    Do not ask for confirmation

Details:
Forcibly destroy the specified controller.  If the API server is accessible,
this command will attempt to destroy the controller model and all hosted models
and their resources.

If the API server is unreachable, the machines of the controller model will be
destroyed through the cloud provisioner.  If there are additional machines,
including machines within hosted models, these machines will not be destroyed
and will never be reconnected to the Juju controller being destroyed.

The normal process of killing the controller will involve watching the hosted
models as they are brought down in a controlled manner. If for some reason the
models do not stop cleanly, there is a default five minute timeout. If no change
in the model state occurs for the duration of this timeout, the command will
stop watching and destroy the models directly through the cloud provider.

See also:
    destroy-controller
    unregister
```