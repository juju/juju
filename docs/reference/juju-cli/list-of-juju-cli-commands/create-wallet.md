(command-juju-create-wallet)=
# `juju create-wallet`

```
Usage: juju create-wallet [options]

Summary:
Create a new wallet.

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
Create a new wallet with monthly limit.

Examples:
    # Creates a wallet named 'qa' with a limit of 42.
    juju create-wallet qa 42
```