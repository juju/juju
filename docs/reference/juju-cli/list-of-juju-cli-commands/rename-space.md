(command-juju-rename-space)=
# `juju rename-space`

```
Usage: juju rename-space [options] <old-name> <new-name>

Summary:
Rename a network space.

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
--rename (= "")
    the new name for the network space

Details:
Renames an existing space from "old-name" to "new-name". Does not change the
associated subnets and "new-name" must not match another existing space.

Examples:

rename a space from db to fe:
	juju rename-space db fe

See also:
	add-space
	list-spaces
	reload-spaces
	remove-space
	show-space
```