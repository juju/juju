(command-juju-grant)=
# `juju grant`

```
Usage: juju grant [options] <user name> <permission> [<model name> ... | <offer url> ...]

Summary:
Grants access level to a Juju user for a model, controller, or application offer.

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
-c, --controller (= "")
    Controller to operate in

Details:
By default, the controller is the current controller.

Users with read access are limited in what they can do with models:
`juju models`, `juju machines`, and `juju status`

Valid access levels for models are:
    read
    write
    admin

Valid access levels for controllers are:
    login
    superuser

Valid access levels for application offers are:
    read
    consume
    admin

Examples:
Grant user 'joe' 'read' access to model 'mymodel':

    juju grant joe read mymodel

Grant user 'jim' 'write' access to model 'mymodel':

    juju grant jim write mymodel

Grant user 'sam' 'read' access to models 'model1' and 'model2':

    juju grant sam read model1 model2

Grant user 'joe' 'read' access to application offer 'fred/prod.hosted-mysql':

    juju grant joe read fred/prod.hosted-mysql

Grant user 'jim' 'consume' access to application offer 'fred/prod.hosted-mysql':

    juju grant jim consume fred/prod.hosted-mysql

Grant user 'sam' 'read' access to application offers 'fred/prod.hosted-mysql' and 'mary/test.hosted-mysql':

    juju grant sam read fred/prod.hosted-mysql mary/test.hosted-mysql

See also:
    revoke
    add-user
```