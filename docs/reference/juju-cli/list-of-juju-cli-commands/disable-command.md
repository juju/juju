(command-juju-disable-command)=
# `juju disable-command`
> See also: [disabled-commands](#disabled-commands), [enable-command](#enable-command)

## Summary
Disables commands for the model.

## Usage
```juju disable-command [options] <command set> [message...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

To prevent the model from being destroyed:

    juju disable-command destroy-model "Check with SA before destruction."

To prevent the machines, applications, units and relations from being removed:

    juju disable-command remove-object

To prevent changes to the model:

    juju disable-command all "Model locked down"


## Details

Juju allows to safeguard deployed models from unintentional damage by preventing
execution of operations that could alter model.

This is done by disabling certain sets of commands from successful execution.
Disabled commands must be manually enabled to proceed.

Some commands offer a `--force` option that can be used to bypass the disabling.

Commands that can be disabled are grouped based on logical operations as follows:

`destroy-model` prevents:

    destroy-controller
    destroy-model

`remove-object` prevents:

    destroy-controller
    destroy-model
    detach-storage
    remove-application
    remove-machine
    remove-relation
    remove-saas
    remove-storage
    remove-unit

`all` prevents:

    add-machine
    integrate
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
    set-application-base
    set-credential
    set-constraints
    sync-agents
    unexpose
    refresh
    upgrade-model
	