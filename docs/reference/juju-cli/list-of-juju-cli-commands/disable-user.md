(command-juju-disable-user)=
# `juju disable-user`

```
Usage: juju disable-user [options] <user name>

Summary:
Disables a Juju user.

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
A disabled Juju user is one that cannot log in to any controller.
This command has no affect on models that the disabled user may have
created and/or shared nor any applications associated with that user.

Examples:
    juju disable-user bob

See also:
    users
    enable-user
    login
```