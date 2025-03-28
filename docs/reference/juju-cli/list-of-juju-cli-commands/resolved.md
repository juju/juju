(command-juju-resolved)=
# `juju resolved`

```
Usage: juju resolved [options] [<unit> ...]

Summary:
Marks unit errors resolved and re-executes failed hooks.

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
--all  (= false)
    Marks all units in error as resolved
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-retry  (= false)
    Do not re-execute failed hooks on the unit

Aliases: resolve
```