(command-juju-remove-saas)=
# `juju remove-saas`

```
Usage: juju remove-saas [options] <saas-application-name> [<saas-application-name>...]

Summary:
Remove consumed applications (SAAS) from the model.

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
    Completely remove a SAAS and all its dependencies
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-wait  (= false)
    Rush through SAAS removal without waiting for each individual step to complete

Details:
Removing a consumed (SAAS) application will terminate any relations that
application has, potentially leaving any related local applications
in a non-functional state.

Examples:
    juju remove-saas hosted-mysql
    juju remove-saas -m test-model hosted-mariadb

See also:
    consume
    offer

Aliases: remove-consumed-application
```