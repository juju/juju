(command-juju-show-space)=
# `juju show-space`

```
Usage: juju show-space [options] <name>

Summary:
Shows information about the network space.

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

Details:
Displays extended information about a given space.
Output includes the space subnets, applications with bindings to the space,
and a count of machines connected to the space.

Examples:

Show a space by name:
	juju show-space alpha

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	remove-space
```