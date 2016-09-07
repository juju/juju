// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

const commandSets = `
Commands that can be disabled are grouped based on logical operations as follows:

"destroy-model" prevents:
    destroy-model

"remove-object" prevents:
    destroy-model
    remove-machine
    remove-relation
    remove-application
    remove-unit

"all" prevents:
    add-machine
    add-relation
    add-unit
    add-ssh-key
    add-user
    change-user-password
    deploy
    disable-user
    destroy-model
    enable-ha
    enable-user
    expose
    import-ssh-key
    remove-application
    remove-machine
    remove-relation
    remove-ssh-key
    remove-unit
    resolved
    retry-provisioning
    run
    set-config
    set-constraints
    set-model-config
    sync-tools
    unexpose
    unset-config
    unset-model-config
    upgrade-charm
    upgrade-juju
	`
