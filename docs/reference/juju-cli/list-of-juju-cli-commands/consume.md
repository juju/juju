(command-juju-consume)=
# `juju consume`

```
Usage: juju consume [options] <remote offer path> [<local application name>]

Summary:
Add a remote offer to the model.

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
Adds a remote offer to the model. Relations can be created later using "juju relate".

The path to the remote offer is formatted as follows:
    [<controller name>:][<model owner>/]<model name>.<application name>

If the controller name is omitted, Juju will use the currently active
controller. Similarly, if the model owner is omitted, Juju will use the user
that is currently logged in to the controller providing the offer.

Examples:
    $ juju consume othermodel.mysql
    $ juju consume owner/othermodel.mysql
    $ juju consume anothercontroller:owner/othermodel.mysql

See also:
    add-relation
    offer
    remove-saas
```