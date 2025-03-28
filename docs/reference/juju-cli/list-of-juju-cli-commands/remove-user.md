(command-juju-remove-user)=
# `juju remove-user`

```
Usage: juju remove-user [options] <user name>

Summary:
Deletes a Juju user from a controller.

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
-y, --yes  (= false)
    Confirm deletion of the user

Details:
This removes a user permanently.

By default, the controller is the current controller.

Examples:
    juju remove-user bob
    juju remove-user bob --yes

See also:
    unregister
    revoke
    show-user
    list-users
    switch-user
    disable-user
    enable-user
    change-user-password
```