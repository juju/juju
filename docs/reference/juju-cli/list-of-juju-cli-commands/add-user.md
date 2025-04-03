(command-juju-add-user)=
# `juju add-user`

```
Usage: juju add-user [options] <user name> [<display name>]

Summary:
Adds a Juju user to a controller.

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
The user's details are stored within the controller and will be removed when
the controller is destroyed.

A user unique registration string will be printed. This registration string
must be used by the newly added user as supplied to complete the registration
process.

Some machine providers will require the user to be in possession of certain
credentials in order to create a model.

Examples:
    juju add-user bob
    juju add-user --controller mycontroller bob

See also:
    register
    grant
    users
    show-user
    disable-user
    enable-user
    change-user-password
    remove-user
```