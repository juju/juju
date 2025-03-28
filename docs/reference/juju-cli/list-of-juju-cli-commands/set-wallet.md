(command-juju-set-wallet)=
# `juju set-wallet`

```
Usage: juju set-wallet [options] <wallet name> <value>

Summary:
Set the wallet limit.

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
Set the monthly wallet limit.

Examples:
    # Sets the monthly limit for wallet named 'personal' to 96.
    juju set-wallet personal 96
```