// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

const commandSets = `
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
    relate
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
	`
