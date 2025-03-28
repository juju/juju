(command-juju-revoke-cloud)=
# `juju revoke-cloud`

```
Usage: juju revoke-cloud [options] <user name> <permission> <cloud name> ...

Summary:
Revokes access from a Juju user for a cloud.

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
Revoking admin access, from a user who has that permission, will leave
that user with add-model access. Revoking add-model access, however, also revokes
admin access.

Valid access levels are:
    admin
    add-model

Examples:
Revoke 'add-model' (and 'admin') access from user 'joe' for cloud 'fluffy':

    juju revoke-cloud joe add-model fluffy

Revoke 'admin' access from user 'sam' for clouds 'fluffy' and 'rainy':

    juju revoke-cloud sam admin fluffy rainy

See also:
    grant-cloud
```