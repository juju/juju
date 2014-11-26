// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

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

Commands that can be unblocked are grouped based on logic operations as follows:

destroy-environment includes command:
    destroy-environment

remove-object includes termination commands:
    remove-machine
    destroy-machine
    terminate-machine
    remove-service
    destroy-service
    remove-unit
    destroy-unit
    remove-relation
    destroy-relation


Examples:
   juju unblock destroy-environment
   #can destroy environment now

   juju unblock remove-object
   #can remove machine, service, unit or relation now


See Also:
   juju help block
`

func (c *UnblockCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unblock",
		Args:    blockArgsFmt,
		Purpose: "unblock an operation that would alter a running environment",
		Doc:     unblockDoc,
	}
}

func (c *UnblockCommand) Init(args []string) error {
	return c.assignValidOperation("unblock", args)
}

func (c *UnblockCommand) Run(_ *cmd.Context) error {
	return c.setBlockEnvironmentVariable(false)
}
