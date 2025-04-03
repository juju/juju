(command-juju-scale-application)=
# `juju scale-application`

```
Usage: juju scale-application [options] <application> <scale>

Summary:
Set the desired number of application units.

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
Scale a k8s application by specifying how many units there should be.
The new number of units can be greater or less than the current number, thus
allowing both scale up and scale down.

Examples:

    juju scale-application mariadb 2
```