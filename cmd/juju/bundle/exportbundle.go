// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package bundle

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/api/bundle"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewexportbundleCommand returns a fully constructed exportbundle command.
func NewExportBundleCommand() cmd.Command {
	return modelcmd.Wrap(&exportBundleCommand{}, modelcmd.WrapSkipModelFlags)
}

type exportBundleCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
	api ExportBundleModelAPI
}

const exportBundleHelpDoc = `
Exports the current model configuration into a YAML file.

Examples:

    juju export-bundle
`

// Info implements Command.
func (c *exportBundleCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "export-bundle",
		Purpose: "Exports the current model configuration in a YAML file.",
		Doc:     exportBundleHelpDoc,
	}
}

// SetFlags implements Command.
func (c *exportBundleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.
func (c *exportBundleCommand) Init(args []string) error {
	modelName := ""
	if len(args) > 0 {
		modelName = args[0]
		args = args[1:]
	}
	if err := c.SetModelName(modelName, true); err != nil {
		return errors.Trace(err)
	}
	if err := c.ModelCommandBase.Init(args); err != nil {
		return err
	}
	return nil
}

// ExportBundleAPI specifies the used function calls of the ModelManager.
type ExportBundleModelAPI interface {
	Close() error
	ExportBundle(names.ModelTag) (params.StringResult, error)
}

func (c *exportBundleCommand) getAPI() (ExportBundleModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}

	api, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bundle.NewClient(api), nil
}

// Run implements Command.
func (c *exportBundleCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "retreiving current model configuration details.")
	}

	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	results, err := client.ExportBundle(modelTag)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, results)
}
