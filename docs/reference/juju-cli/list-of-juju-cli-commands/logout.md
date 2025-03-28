(command-juju-logout)=
# `juju logout`

```
Usage: juju logout [options]

Summary:
Logs a Juju user out of a controller.

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
--force  (= false)
    Force logout when a locally recorded password is detected

Details:
If another client has logged in as the same user, they will remain logged
in. This command only affects the local client.

The command will fail if the user has not yet set a password
(`juju change-user-password`). This scenario is only possible after
`juju bootstrap`since `juju register` sets a password. The
failing behaviour can be overridden with the '--force' option.

If the same user is logged in with another client system, that user session
will not be affected by this command; it only affects the local client.

By default, the controller is the current controller.

Examples:
    juju logout

See also:
    change-user-password
    login
```