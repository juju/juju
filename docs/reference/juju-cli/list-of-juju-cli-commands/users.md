(command-juju-users)=
# `juju users`

```
Usage: juju users [options] [model-name]

Summary:
Lists Juju users allowed to connect to a controller or model.

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
--all  (= false)
    Include disabled users
-c, --controller (= "")
    Controller to operate in
--exact-time  (= false)
    Use full timestamp for connection times
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file

Details:
When used without a model name argument, users relevant to a controller are printed.
When used with a model name, users relevant to the specified model are printed.

Examples:
    Print the users relevant to the current controller:
    juju users

    Print the users relevant to the controller "another":
    juju users -c another

    Print the users relevant to the model "mymodel":
    juju users mymodel

See also:
    add-user
    register
    show-user
    disable-user
    enable-user

Aliases: list-users
```