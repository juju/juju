(command-juju-models)=
# `juju models`

```
Usage: juju models [options]

Summary:
Lists models a user can access on a controller.

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
--all  (= false)
    Lists all models, regardless of user accessibility (administrative users only)
-c, --controller (= "")
    Controller to operate in
--exact-time  (= false)
    Use full timestamps
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file
--user (= "")
    The user to list models for (administrative users only)
--uuid  (= false)
    Display UUID for models

Details:
The models listed here are either models you have created yourself, or
models which have been shared with you. Default values for user and
controller are, respectively, the current user and the current controller.
The active model is denoted by an asterisk.

Examples:

    juju models
    juju models --user bob

See also:
    add-model
    share-model
    unshare-model

Aliases: list-models
```