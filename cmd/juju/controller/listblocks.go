// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewListBlocksCommand returns a command to list the blocks in a controller.
func NewListBlocksCommand() cmd.Command {
	return modelcmd.WrapController(&listBlocksCommand{})
}

// listBlocksCommand lists all blocks for environments within the controller.
type listBlocksCommand struct {
	modelcmd.ControllerCommandBase
	out    cmd.Output
	api    listBlocksAPI
	apierr error
}

var listBlocksDoc = `List all blocks for models within the specified controller`

// listBlocksAPI defines the methods on the controller API endpoint
// that the list-blocks command calls.
type listBlocksAPI interface {
	Close() error
	ListBlockedModels() ([]params.ModelBlockInfo, error)
}

// Info implements Command.Info.
func (c *listBlocksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-all-blocks",
		Purpose: "list all blocks within the controller",
		Doc:     listBlocksDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *listBlocksCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabularBlockedEnvironments,
	})
}

func (c *listBlocksCommand) getAPI() (listBlocksAPI, error) {
	if c.api != nil {
		return c.api, c.apierr
	}
	return c.NewControllerAPIClient()
}

// Run implements Command.Run
func (c *listBlocksCommand) Run(ctx *cmd.Context) error {
	api, err := c.getAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API")
	}
	defer api.Close()

	envs, err := api.ListBlockedModels()
	if err != nil {
		logger.Errorf("Unable to list blocked models: %s", err)
		return err
	}
	return c.out.Write(ctx, envs)
}
