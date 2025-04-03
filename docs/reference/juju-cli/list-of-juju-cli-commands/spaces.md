(command-juju-spaces)=
# `juju spaces`

```
Usage: juju spaces [options] [--short] [--format yaml|json] [--output <path>]

Summary:
List known spaces, including associated subnets.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--short  (= false)
    only display spaces.

Details:
Displays all defined spaces. By default both spaces and their subnets are displayed.
Supplying the --short option will list just the space names.
The --output argument allows the command's output to be redirected to a file.

Examples:

List spaces and their subnets:

	juju spaces

List spaces:

	juju spaces --short

See also:
	add-space
	reload-spaces

Aliases: list-spaces
```