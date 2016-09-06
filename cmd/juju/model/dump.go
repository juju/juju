// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewDumpCommand returns a fully constructed dump-model command.
func NewDumpCommand() cmd.Command {
	return modelcmd.WrapController(&dumpCommand{})
}

type dumpCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output
	api DumpModelAPI

	model string
}

const dumpModelHelpDoc = `
Calls export on the model's database representation and writes the
resulting YAML to stdout.

Examples:

    juju dump-model
    juju dump-model mymodel

See also:
    models
`

// Info implements Command.
func (c *dumpCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "dump-model",
		Args:    "[model-name]",
		Purpose: "Displays the database agnostic representation of the model.",
		Doc:     dumpModelHelpDoc,
	}
}

// SetFlags implements Command.
func (c *dumpCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.
func (c *dumpCommand) Init(args []string) error {
	if len(args) == 1 {
		c.model = args[0]
		return nil
	}
	return cmd.CheckEmpty(args)
}

// DumpModelAPI specifies the used function calls of the ModelManager.
type DumpModelAPI interface {
	Close() error
	DumpModel(names.ModelTag) (map[string]interface{}, error)
}

func (c *dumpCommand) getAPI() (DumpModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

// Run implements Command.
func (c *dumpCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	store := c.ClientStore()
	if c.model == "" {
		c.model, err = store.CurrentModel(c.ControllerName())
		if err != nil {
			return err
		}
	}

	modelDetails, err := store.ModelByName(
		c.ControllerName(),
		c.model,
	)
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	results, err := client.DumpModel(modelTag)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, results)
}
