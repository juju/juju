(command-juju-grant-cloud)=
# `juju grant-cloud`

```
Usage: juju grant-cloud [options] <user name> <permission> <cloud name> ...

Summary:
Grants access level to a Juju user for a cloud.

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
Valid access levels are:
    admin
    add-model

Examples:
Grant user 'joe' 'add-model' access to cloud 'fluffy':

    juju grant-cloud joe add-model fluffy

See also:
    revoke-cloud
    add-user
```