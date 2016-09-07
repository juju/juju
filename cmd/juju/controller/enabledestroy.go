// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewEnableDestroyControllerCommand returns a command that allows a controller admin
// to remove blocks from the controller.
func NewEnableDestroyControllerCommand() cmd.Command {
	return modelcmd.WrapController(&enableDestroyController{})
}

type enableDestroyController struct {
	modelcmd.ControllerCommandBase
	api removeBlocksAPI
}

type removeBlocksAPI interface {
	Close() error
	RemoveBlocks() error
}

var removeBlocksDoc = `
Any model in the controller that has disabled-commands will block a controller
from being destroyed.

A controller administrator is able to enable all the commands across all the models
in a Juju controller.

See Also:
    juju disable-command
    juju disabled-commands
    juju enable-command
`

// Info implements Command.Info
func (c *enableDestroyController) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "enable-destroy-controller",
		Purpose: "Enable all commands on all models in the controller.",
		Doc:     removeBlocksDoc,
	}
}

func (c *enableDestroyController) getAPI() (removeBlocksAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewControllerAPIClient()
}

// Run implements Command.Run
func (c *enableDestroyController) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	return errors.Trace(client.RemoveBlocks())
}
