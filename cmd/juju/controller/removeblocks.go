// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveBlocksCommand returns a command that allows a controller admin
// to remove blocks from the controller.
func NewRemoveBlocksCommand() cmd.Command {
	return modelcmd.WrapController(&removeBlocksCommand{})
}

type removeBlocksCommand struct {
	modelcmd.ControllerCommandBase
	api removeBlocksAPI
}

type removeBlocksAPI interface {
	Close() error
	RemoveBlocks() error
}

var removeBlocksDoc = `
Remove all blocks in the Juju controller.

A controller administrator is able to remove all the blocks that have been added
in a Juju controller.

See Also:
    juju help block
    juju help unblock
`

// Info implements Command.Info
func (c *removeBlocksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-all-blocks",
		Purpose: "remove all blocks in the Juju controller",
		Doc:     removeBlocksDoc,
	}
}

func (c *removeBlocksCommand) getAPI() (removeBlocksAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewControllerAPIClient()
}

// Run implements Command.Run
func (c *removeBlocksCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	return errors.Annotatef(client.RemoveBlocks(), "cannot remove blocks")
}
