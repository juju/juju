(command-juju-show-application)=
# `juju show-application`

```
Usage: juju show-application [options] <application name or alias>

Summary:
Displays information about an application.

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
    Specify output format (json|smart|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
The command takes deployed application names or aliases as an argument.

The command does an exact search. It does not support wildcards.

Examples:
    juju show-application mysql
    juju show-application mysql wordpress

    juju show-application myapplication
      where "myapplication" is the application name alias, see "juju help deploy" for more information
```