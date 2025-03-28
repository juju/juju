(command-juju-run-action)=
# `juju run-action`

```
Usage: juju run-action [options] <unit> [<unit> ...] <action> [<key>=<value> [<key>[.<key> ...]=<value>]]

Summary:
Queue an action for execution.

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
--params  (= )
    Path to yaml-formatted params file
--string-args  (= false)
    Use raw string values of CLI args
--wait  (= )
    Wait for results, with optional timeout

Details:
Queue an Action for execution on a given unit, with a given set of params.
The Action ID is returned for use with 'juju show-action-output <ID>' or
'juju show-action-status <ID>'.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If the leader syntax is used, the leader unit for the application will be
resolved before the action is enqueued.

Params are validated according to the charm for the unit's application.  The
valid params can be seen using "juju actions <application> --schema".
Params may be in a yaml file which is passed with the --params option, or they
may be specified by a key.key.key...=value format (see examples below.)

Params given in the CLI invocation will be parsed as YAML unless the
--string-args option is set.  This can be helpful for values such as 'y', which
is a boolean true in YAML.

If --params is passed, along with key.key...=value explicit arguments, the
explicit arguments will override the parameter file.

Examples:

    juju run-action mysql/3 backup --wait
    juju run-action mysql/3 backup
    juju run-action mysql/leader backup
    juju show-action-output <ID>
    juju run-action mysql/3 backup --params parameters.yml
    juju run-action mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
    juju run-action mysql/3 backup --params p.yml file.kind=xz file.quality=high
    juju run-action sleeper/0 pause time=1000
    juju run-action sleeper/0 pause --string-args time=1000
```