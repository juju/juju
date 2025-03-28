(command-juju-add-space)=
# `juju add-space`

```
Usage: juju add-space [options] <name> [<CIDR1> <CIDR2> ...]

Summary:
Add a new network space.

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
Adds a new space with the given name and associates the given
(optional) list of existing subnet CIDRs with it.

```