(command-juju-show-wallet)=
# `juju show-wallet`

```
Usage: juju show-wallet [options] <wallet>

Summary:
Show details about a wallet.

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
--format  (= tabular)
    Specify output format (json|tabular)
-o, --output (= "")
    Specify an output file

Details:
Display wallet usage information.

Examples:
    juju show-wallet personal
```