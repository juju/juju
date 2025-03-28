(command-juju-bind)=
# `juju bind`

```
Usage: juju bind [options] <application> [<default-space>] [<endpoint-name>=<space> ...]

Summary:
Change bindings for a deployed application.

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
    Allow endpoints to be bound to spaces that might not be available to all existing units
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
In order to be able to bind any endpoint to a space, all machines where the
application units are deployed to are required to be configured with an address
in that space. However, you can use the --force option to bypass this check.

Examples:

To update the default binding for the application and automatically update all
existing endpoint bindings that were referencing the old default, you can use
the following syntax:

  juju bind foo new-default

To bind individual endpoints to a space you can use the following syntax:

  juju bind foo endpoint-1=space-1 endpoint-2=space-2

Finally, the above commands can be combined to update both the default space
and individual endpoints in one go:

  juju bind foo new-default endpoint-1=space-1
```