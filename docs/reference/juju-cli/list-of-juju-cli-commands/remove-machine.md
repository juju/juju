(command-juju-remove-machine)=
# `juju remove-machine`

```
Usage: juju remove-machine [options] <machine number> ...

Summary:
Removes one or more machines from a model.

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
--force  (= false)
    Completely remove a machine and all its dependencies
--keep-instance  (= false)
    Do not stop the running cloud instance
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-prompt  (= false)
    Does nothing. Option present for forward compatibility with Juju 3
--no-wait  (= false)
    Rush through machine removal without waiting for each individual step to complete

Details:
Machines are specified by their numbers, which may be retrieved from the
output of `juju status`.

It is possible to remove machine from Juju model without affecting
the corresponding cloud instance by using --keep-instance option.

Machines responsible for the model cannot be removed.

Machines running units or containers can be removed using the '--force'
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using --force, users can also specify --no-wait to progress through steps
without delay waiting for each step to complete.

Examples:

    juju remove-machine 5
    juju remove-machine 6 --force
    juju remove-machine 6 --force --no-wait
    juju remove-machine 7 --keep-instance

See also:
    add-machine
```