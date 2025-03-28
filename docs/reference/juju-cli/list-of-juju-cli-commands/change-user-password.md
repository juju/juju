(command-juju-change-user-password)=
# `juju change-user-password`

```
Usage: juju change-user-password [options] [username]

Summary:
Changes the password for the current or specified Juju user.

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
-c, --controller (= "")
    Controller to operate in
--no-prompt  (= false)
    don't prompt for password and just read a line from stdin
--reset  (= false)
    Reset user password

Details:
The user is, by default, the current user. The latter can be confirmed with
the `juju show-user` command.

If no controller is specified, the current controller will be used.

A controller administrator can change the password for another user
by providing desired username as an argument.

A controller administrator can also reset the password with a --reset option.
This will invalidate any passwords that were previously set
and registration strings that were previously issued for a user.
This option will issue a new registration string to be used with
`juju register`.


Examples:

    juju change-user-password
    juju change-user-password bob
    juju change-user-password bob --reset
    juju change-user-password -c another-known-controller
    juju change-user-password bob --controller another-known-controller

See also:
    add-user
    register

```