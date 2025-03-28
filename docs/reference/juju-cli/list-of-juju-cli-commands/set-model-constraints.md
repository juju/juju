(command-juju-set-model-constraints)=
# `juju set-model-constraints`

```
Usage: juju set-model-constraints [options] <constraint>=<value> ...

Summary:
Sets machine constraints on a model.

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
Sets constraints on the model that can be viewed with
`juju get-model-constraints`.  By default, the model is the current model.
Model constraints are combined with constraints set for an application with
`juju set-constraints` for commands (such as 'deploy') that provision
machines/containers for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific application can be viewed with `juju get-constraints`.

Examples:

    juju set-model-constraints cores=8 mem=16G
    juju set-model-constraints -m mymodel root-disk=64G

See also:
    models
    get-model-constraints
    get-constraints
    set-constraints
```