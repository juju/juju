// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/internal/cmd"
)

// NewDumpCommand returns a fully constructed dump-model command.
func NewDumpCommand() cmd.Command {
	return modelcmd.Wrap(&dumpCommand{})
}

type dumpCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
	api DumpModelAPI
}

const dumpModelHelpDoc = `
Calls export on the model's database representation and writes the
resulting YAML to stdout.

Examples:

    juju dump-model
    juju dump-model -m mymodel

See also:
    models
`

// Info implements Command.
func (c *dumpCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "dump-model",
		Purpose: "Displays the database agnostic representation of the model.",
		Doc:     dumpModelHelpDoc,
	})
}

// SetFlags implements Command.
func (c *dumpCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.
func (c *dumpCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// DumpModelAPI specifies the used function calls of the ModelManager.
type DumpModelAPI interface {
	Close() error
	DumpModel(context.Context, names.ModelTag) (map[string]interface{}, error)
}

func (c *dumpCommand) getAPI(ctx context.Context) (DumpModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.ModelCommandBase.NewModelManagerAPIClient(ctx)
}

// Run implements Command.
func (c *dumpCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails(ctx)
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	results, err := client.DumpModel(ctx, modelTag)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, results)
}
