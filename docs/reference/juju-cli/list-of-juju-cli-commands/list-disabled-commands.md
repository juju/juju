(command-juju-list-disabled-commands)=
# `juju list-disabled-commands`
> See also: [disable-command](#disable-command), [enable-command](#enable-command)

**Aliases:** list-disabled-commands

## Summary
Lists disabled commands.

## Usage
```juju disabled-commands [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--all` | false | Lists for all models (administrative users only) |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Details

Lists disabled commands for the model.

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
	