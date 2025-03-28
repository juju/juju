(command-juju-sla)=
# `juju sla`

```
Usage: juju sla [options] <level>

Summary:
Set the SLA level for a model.

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
--budget (= "")
    the maximum spend for the model
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
Set the support level for the model, effective immediately.
Warning: this command is DEPRECATED and no longer supported.

Examples:
    # set the support level to essential
    juju sla essential

    # set the support level to essential with a maximum budget of $1000 in wallet 'personal'
    juju sla standard --budget personal:1000

    # display the current support level for the model.
    juju sla
```