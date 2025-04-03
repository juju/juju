(command-juju-agreements)=
# `juju agreements`

```
Usage: juju agreements [options]

Summary:
List user's agreements.

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
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file

Details:
Charms may require a user to accept its terms in order for it to be deployed.
In other words, some applications may only be installed if a user agrees to
accept some terms defined by the charm.

This command lists the terms that the user has agreed to.

See also:
    agree

Aliases: list-agreements
```