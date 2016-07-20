// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/cmd/modelcmd"
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
	return &cmd.Info{
		Name:    "dump-model",
		Purpose: "Displays the database agnostic representation of the model.",
		Doc:     strings.TrimSpace(dumpModelHelpDoc),
	}
}

// SetFlags implements Command.
func (c *dumpCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements Command.
func (c *dumpCommand) Init(args []string) (err error) {
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
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
}

// Run implements Command.
func (c *dumpCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	store := c.ClientStore()
	modelDetails, err := store.ModelByName(
		c.ControllerName(),
		c.ModelName(),
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
