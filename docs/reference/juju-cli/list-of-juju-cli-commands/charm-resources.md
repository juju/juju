(command-juju-charm-resources)=
# `juju charm-resources`

```
Usage: juju charm-resources [options] <charm>

Summary:
Display the resources for a charm in a repository.

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
--channel (= "stable")
    the channel of the charm
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
This command will report the resources and the current revision of each
resource for a charm in a repository.

<charm> can be a charm URL, or an unambiguously condensed form of it,
just like the deploy command.

Release is implied from the <charm> supplied. If not provided, the default
series for the model is used.

Channel can be specified with --channel.  If not provided, stable is used.

Where a channel is not supplied, stable is used.

Examples:

Display charm resources for the postgresql charm:
    juju charm-resources postgresql

Display charm resources for mycharm in the 2.0/edge channel:
    juju charm-resources mycharm --channel 2.0/edge

Aliases: list-charm-resources
```