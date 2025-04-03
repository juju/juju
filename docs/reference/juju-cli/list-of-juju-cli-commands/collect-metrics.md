(command-juju-collect-metrics)=
# `juju collect-metrics`

```
Usage: juju collect-metrics [options] [application or unit]

Summary:
Collect metrics on the given unit/application.

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
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
Trigger metrics collection

This command waits for the metric collection to finish before returning.
You may abort this command and it will continue to run asynchronously.
Results may be checked by 'juju show-action-status'.
```