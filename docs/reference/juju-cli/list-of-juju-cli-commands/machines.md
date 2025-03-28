(command-juju-machines)=
# `juju machines`

```
Usage: juju machines [options]

Summary:
Lists machines in a model.

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
--color  (= false)
    Force use of ANSI color codes
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--utc  (= false)
    Display time as UTC in RFC3339 format

Details:
By default, the tabular format is used.
The following sections are included: ID, STATE, DNS, INS-ID, SERIES, AZ
Note: AZ above is the cloud region's availability zone.

Examples:
     juju machines

See also:
    status

Aliases: list-machines
```