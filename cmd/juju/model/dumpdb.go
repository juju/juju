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

// NewDumpDBCommand returns a fully constructed dump-db command.
func NewDumpDBCommand() cmd.Command {
	return modelcmd.WrapController(&dumpDBCommand{})
}

type dumpDBCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output
	api DumpDBAPI

	model string
}

const dumpDBHelpDoc = `
dump-db returns all that is stored in the database for the specified model.

Examples:

    juju dump-db
    juju dump-db mymodel

See also:
    models
`

// Info implements Command.
func (c *dumpDBCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "dump-db",
		Args:    "[model-name]",
		Purpose: "Displays the mongo documents for of the model.",
		Doc:     dumpDBHelpDoc,
	}
}

// SetFlags implements Command.
func (c *dumpDBCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.
func (c *dumpDBCommand) Init(args []string) error {
	if len(args) == 1 {
		c.model = args[0]
		return nil
	}
	return cmd.CheckEmpty(args)
}

// DumpDBAPI specifies the used function calls of the ModelManager.
type DumpDBAPI interface {
	Close() error
	DumpModelDB(names.ModelTag) (map[string]interface{}, error)
}

func (c *dumpDBCommand) getAPI() (DumpDBAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

// Run implements Command.
func (c *dumpDBCommand) Run(ctx *cmd.Context) error {
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
	results, err := client.DumpModelDB(modelTag)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, results)
}
