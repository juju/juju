> See also: [disable-command](#disable-command), [disabled-commands](#disabled-commands)

## Summary
Enable commands that had been previously disabled.

## Usage
```juju enable-command [options] <command set>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

To allow the model to be destroyed:

    juju enable-command destroy-model

To allow the machines, applications, units and relations to be removed:

    juju enable-command remove-object

To allow changes to the model:

    juju enable-command all


## Details

Juju allows to safeguard deployed models from unintentional damage by preventing
execution of operations that could alter model.

This is done by disabling certain sets of commands from successful execution.
Disabled commands must be manually enabled to proceed.

Some commands offer a --force option that can be used to bypass a block.

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
	



