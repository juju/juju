// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
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

var enableDestroyDoc = `
Any model in the controller that has disabled commands will block a controller
from being destroyed.

A controller administrator can enable all the commands across all the models
in a Juju controller so that the controller can be destroyed if desired.
`

// Info implements Command.Info
func (c *enableDestroyController) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "enable-destroy-controller",
		Purpose: "Enable destroy-controller by removing disabled commands in the controller.",
		Doc:     enableDestroyDoc,
		SeeAlso: []string{
			"disable-command",
			"disabled-commands",
			"enable-command",
		},
	})
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
