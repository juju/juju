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
must be manually unblocked to proceed. Although, few operations that offer --force option can use it to by-pass a block.

Operations that can be unblocked are

destroy environment
termination commands


Examples:
   juju unblock destroy-environment
   (unblocks destroy environment)

   juju unblock remove-object
   (unblocks all remove commands: remove-machine, remove-service, remove-unit, remove-relation)

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
