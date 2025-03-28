(command-juju-remove-space)=
# `juju remove-space`

```
Usage: juju remove-space [options] <name>

Summary:
Remove a network space.

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
--force  (= false)
    remove the offer as well as any relations to the offer
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-y, --yes  (= false)
    Do not prompt for confirmation

Details:
Removes an existing Juju network space with the given name. Any subnets
associated with the space will be transferred to the default space.
The command will fail if existing constraints, bindings or controller settings are bound to the given space.

If the --force option is specified, the space will be deleted even if there are existing bindings, constraints or settings.

Examples:

Remove a space by name:
	juju remove-space db-space

Remove a space by name with force, without need for confirmation:
	juju remove-space db-space --force -y

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	show-space
```