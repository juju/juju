// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import "github.com/juju/cmd"

// UnblockCommand removes the block from desired operation.
type UnblockCommand struct {
	ProtectionCommand
}

var unblockDoc = `

Juju allows to safeguard deployed environments from unintentional damage by preventing
execution of operations that could alter environment.

This is done by blocking certain operations from successful execution. Blocked operations
must be manually unblocked to proceed.

Some comands offer a --force option that can be used to bypass a block.

Commands that can be unblocked are grouped based on logical operations as follows:

destroy-environment includes command:
    destroy-environment

remove-object includes termination commands:
    destroy-environment
    remove-machine
    remove-relation
    remove-service
    remove-unit

all-changes includes all alteration commands
    add-machine
    add-relation
    add-unit
    authorised-keys add
    authorised-keys delete
    authorised-keys import
    deploy
    destroy-environment
    expose
    remove-machine
    remove-relation
    remove-service
    remove-unit
    resolved
    retry-provisioning
    run
    set
    set-constraints
    set-env
    unexpose
    unset
    unset-env
    upgrade-charm
    upgrade-juju
    user add
    user change-password
    user disable
    user enable

Examples:
   To allow the environment to be destroyed:
   juju unblock destroy-environment

   To allow the machines, services, units and relations to be removed:
   juju unblock remove-object

   To allow changes to the environment:
   juju unblock all-changes

See Also:
   juju help block
`

// Info provides information about command.
// Satisfying Command interface.
func (c *UnblockCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unblock",
		Args:    blockArgsFmt,
		Purpose: "unblock an operation that would alter a running environment",
		Doc:     unblockDoc,
	}
}

// Init initializes the command.
// Satisfying Command interface.
func (c *UnblockCommand) Init(args []string) error {
	return c.assignValidOperation("unblock", args)
}

// Run unblocks previously blocked commands.
// Satisfying Command interface.
func (c *UnblockCommand) Run(_ *cmd.Context) error {
	return c.setBlockEnvironmentVariable(false)
}
