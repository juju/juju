(command-juju-show-model)=
# `juju show-model`

```
Usage: juju show-model [options] <model name>

Summary:
Shows information about the current or specified model.

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
--format  (= yaml)
    Specify output format (json|yaml)
-o, --output (= "")
    Specify an output file

Details:
Show information about the current or specified model.
```