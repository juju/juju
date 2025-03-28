(command-juju-move-to-space)=
# `juju move-to-space`

```
Usage: juju move-to-space [options] [--format yaml|json] <name> <CIDR1> [ <CIDR2> ...]

Summary:
Update a network space's CIDR.

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
--force  (= false)
    Allow to force a move of subnets to a space even if they are in use on another machine.
--format  (= human)
    Specify output format (human|json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
Replaces the list of associated subnets of the space. Since subnets
can only be part of a single space, all specified subnets (using their
CIDRs) "leave" their current space and "enter" the one we're updating.

Examples:

Move a list of CIDRs from their space to a new space:
	juju move-to-space db-space 172.31.1.0/28 172.31.16.0/20

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	show-space
	remove-space
```