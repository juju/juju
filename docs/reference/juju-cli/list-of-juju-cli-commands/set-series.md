(command-juju-set-series)=
# `juju set-series`

```
Usage: juju set-series [options] <application> <series>

Summary:
Set an application's series.

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
The specified application's series value will be set within juju. Any subordinates of
the application will also have their series set to the provided value.

This will not change the series of any existing units, rather new units will use
the new series when deployed.

It is recommended to only do this after upgrade-series has been run for machine containing
all existing units of the application.

To ensure correct binaries, run 'juju refresh' before running 'juju add-unit'.

Examples:

Set the series for the ubuntu application to focal

	juju set-series ubuntu focal

See also:
    status
    refresh
    upgrade-series

Aliases: set-application-base
```