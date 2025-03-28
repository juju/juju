(command-juju-exec)=
# `juju exec`

```
Usage: juju exec [options] <commands>

Summary:
Run the commands on the remote targets specified.

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
-a, --app, --application  (= )
    One or more application names
--all  (= false)
    Run the commands on all the machines
--format  (= default)
    Specify output format (default|json|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--machine  (= )
    One or more machine ids
-o, --output (= "")
    Specify an output file
--operator  (= false)
    Run the commands on the operator (k8s-only)
--timeout  (= 5m0s)
    How long to wait before the remote command is considered to have failed
-u, --unit  (= )
    One or more unit ids

Details:
Run a shell command on the specified targets. Only admin users of a model
are able to use this command.

Targets are specified using either machine ids, application names or unit
names.  At least one target specifier is needed.

Multiple values can be set for --machine, --application, and --unit by using
comma separated values.

Depending on the type of target, the user which the command runs as will be:
  unit -> "root"
  machine -> "ubuntu"
The target and user are independent of whether --all or --application are used.
For example, --all will run as "ubuntu" on machines and "root" on units.
And --application will run as "root" on all units of that application.

Some options are shortened for usabilty purpose in CLI
--application can also be specified as --app and -a
--unit can also be specified as -u

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If the target is an application, the command is run on all units for that
application. For example, if there was an application "mysql" and that application
had two units, "mysql/0" and "mysql/1", then
  --application mysql
is equivalent to
  --unit mysql/0,mysql/1

If --operator is provided on k8s models, commands are executed on the operator
instead of the workload. On IAAS models, --operator has no effect.

Commands run for applications or units are executed in a 'hook context' for
the unit.

--all is provided as a simple way to run the command on all the machines
in the model.  If you specify --all you cannot provide additional
targets.

Since juju exec creates actions, you can query for the status of commands
started with juju run by calling "juju show-action-status --name juju-run".

If you need to pass options to the command being run, you must precede the
command and its arguments with "--", to tell "juju exec" to stop processing
those arguments. For example:

    juju exec --all -- hostname -f
```