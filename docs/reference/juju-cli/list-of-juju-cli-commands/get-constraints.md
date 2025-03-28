(command-juju-get-constraints)=
# `juju get-constraints`

```
Usage: juju get-constraints [options] <application>

Summary:
Displays machine constraints for an application.

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
--format  (= constraints)
    Specify output format (constraints|json|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
Shows machine constraints that have been set for an application with `juju set-
constraints`.
By default, the model is the current model.
Application constraints are combined with model constraints, set with `juju
set-model-constraints`, for commands (such as 'deploy') that provision
machines for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific model can be viewed with `juju get-model-
constraints`.

Examples:
    juju get-constraints mysql
    juju get-constraints -m mymodel apache2

See also:
    set-constraints
    get-model-constraints
    set-model-constraints
```