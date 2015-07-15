// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/envcmd"
)

// RemoveBlocksCommand returns the list of all systems the user is
// currently logged in to on the current machine.
type RemoveBlocksCommand struct {
	envcmd.SysCommandBase
	api removeBlocksAPI
}

type removeBlocksAPI interface {
	Close() error
	RemoveBlocks() error
}

var removeBlocksDoc = `
Remove all blocks in the Juju system.

A system administrator is able to remove all the blocks that have been added
in a Juju system.

See Also:
    juju help block
    juju help unblock
`

// Info implements Command.Info
func (c *RemoveBlocksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-blocks",
		Purpose: "remove all blocks in the Juju system",
		Doc:     removeBlocksDoc,
	}
}

func (c *RemoveBlocksCommand) getAPI() (removeBlocksAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewSystemManagerAPIClient()
}

// Run implements Command.Run
func (c *RemoveBlocksCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	return errors.Annotatef(client.RemoveBlocks(), "cannot remove blocks")
}
