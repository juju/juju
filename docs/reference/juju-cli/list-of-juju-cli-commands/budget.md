(command-juju-budget)=
# `juju budget`

```
Usage: juju budget [options] [<wallet>:]<limit>

Summary:
Update a budget.

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
--model-uuid (= "")
    Model uuid to set budget for.

Details:
Updates an existing budget for a model.

Examples:
    # Sets the budget for the current model to 10.
    juju budget 10
    # Moves the budget for the current model to wallet 'personal' and sets the limit to 10.
    juju budget personal:10
```