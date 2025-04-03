(command-juju-get-model-constraints)=
# `juju get-model-constraints`

```
Usage: juju get-model-constraints [options]

Summary:
Displays machine constraints for a model.

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
Shows constraints that have been set on the model with
`juju set-model-constraints.`
By default, the model is the current model.
Model constraints are combined with constraints set on an application
with `juju set-constraints` for commands (such as 'deploy') that provision
machines/containers for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific application can be viewed with `juju get-constraints`.

Examples:

    juju get-model-constraints
    juju get-model-constraints -m mymodel

See also:
    models
    get-constraints
    set-constraints
    set-model-constraints
```