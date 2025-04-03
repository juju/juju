(command-juju-enable-user)=
# `juju enable-user`

```
Usage: juju enable-user [options] <user name>

Summary:
Re-enables a previously disabled Juju user.

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
An enabled Juju user is one that can log in to a controller.

Examples:
    juju enable-user bob

See also:
    users
    disable-user
    login

```