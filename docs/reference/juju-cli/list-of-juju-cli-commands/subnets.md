(command-juju-subnets)=
# `juju subnets`

```
Usage: juju subnets [options] [--space <name>] [--zone <name>] [--format yaml|json] [--output <path>]

Summary:
List subnets known to Juju.

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
    Specify output format (json|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--space (= "")
    Filter results by space name
--zone (= "")
    Filter results by zone name

Details:
Displays a list of all subnets known to Juju. Results can be filtered
using the optional --space and/or --zone arguments to only display
subnets associated with a given network space and/or availability zone.

Like with other Juju commands, the output and its format can be changed
using the --format and --output (or -o) optional arguments. Supported
output formats include "yaml" (default) and "json". To redirect the
output to a file, use --output.

Aliases: list-subnets
```