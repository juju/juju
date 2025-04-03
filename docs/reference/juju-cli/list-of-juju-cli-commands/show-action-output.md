(command-juju-show-action-output)=
# `juju show-action-output`

```
Usage: juju show-action-output [options] <action>

Summary:
Show results of an action.

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
    Specify output format (json|plain|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--utc  (= false)
    Show times in UTC
--wait (= "-1s")
    Wait for results
--watch  (= false)
    Wait indefinitely for results

Details:
Show the results returned by an action with the given ID.
To block until the result is known completed or failed, use
the --wait option with a duration, as in --wait 5s or --wait 1h.
Use --watch to wait indefinitely.

The default behavior without --wait or --watch is to immediately check and return;
if the results are "pending" then only the available information will be
displayed.  This is also the behavior when any negative time is given.

Note: if Juju has been upgraded from 2.6 and there are old action UUIDs still in use,
and you want to specify just the UUID prefix to match on, you will need to include up
to at least the first "-" to disambiguate from a newer numeric id.

Examples:

    juju show-action-output 1
    juju show-action-output 1 --wait=2m
    juju show-action-output 1 --watch

See also:
    run-action
    list-operations
    show-operation
```