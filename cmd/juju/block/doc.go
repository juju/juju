// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package block provides the ability to block certain operations on a model.
// This is useful when you want to prevent accidental changes to a model.
// The operations that can be blocked are grouped into logical sets, and
// each set can be enabled or disabled independently.
//
// Commands that interact with the model should check if the operation they
// performed is blocked. A user friendly client message explaining the block
// should be provided to the user from the API server.
// Tests should be written to ensure that the correct operations are blocked
// when the block is enabled.

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
	`
