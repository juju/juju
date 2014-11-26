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

Commands that can be unblocked are grouped based on logical operations as follows:

destroy-environment includes command:
    destroy-environment

remove-object includes termination commands:
    remove-machine
    remove-service
    remove-unit
    remove-relation


Examples:
   To allow the environment to be destroyed:
   juju unblock destroy-environment

   To allow the machines, services, units and relations to be removed:
   juju unblock remove-object


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
