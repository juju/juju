(command-juju-disabled-commands)=
# `juju disabled-commands`

```
Usage: juju disabled-commands [options]

Summary:
List disabled commands.

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
    Lists for all models (administrative users only)
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
List disabled commands for the model.

Commands that can be disabled are grouped based on logical operations as follows:

"destroy-model" prevents:
    destroy-controller
    destroy-model

"remove-object" prevents:
    destroy-controller
    destroy-model
    detach-storage
    remove-application
    remove-machine
    remove-relation
    remove-saas
    remove-storage
    remove-unit

"all" prevents:
    add-machine
    add-relation
    add-unit
    add-ssh-key
    add-user
    attach-resource
    attach-storage
    change-user-password
    config
    consume
    deploy
    destroy-controller
    destroy-model
    disable-user
    enable-ha
    enable-user
    expose
    import-filesystem
    import-ssh-key
    model-defaults
    model-config
    reload-spaces
    remove-application
    remove-machine
    remove-relation
    remove-ssh-key
    remove-unit
    remove-user
    resolved
    retry-provisioning
    run
    scale-application
    set-credential
    set-constraints
    set-series
    sync-agents
    unexpose
    upgrade-charm
    upgrade-model

See also:
    disable-command
    enable-command

Aliases: list-disabled-commands
```