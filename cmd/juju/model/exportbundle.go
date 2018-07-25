// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/bundle"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewExportBundleCommand returns a fully constructed export bundle command.
func NewExportBundleCommand() cmd.Command {
	cmd := &exportBundleCommand{}
	cmd.newAPIFunc = func() (ExportBundleAPI, error) {
		return cmd.getAPI()
	}
	return modelcmd.Wrap(cmd)
}

type exportBundleCommand struct {
	modelcmd.ModelCommandBase
	out        cmd.Output
	newAPIFunc func() (ExportBundleAPI, error)
	Filename   string
}

const exportBundleHelpDoc = `
Exports the current model configuration as bundle.

--filename specifies the file where bundle is written to. Its optional.
If --filename is not used, the bundle is displayed in stdout.

Examples:

    juju export-bundle
	juju export-bundle --filename mymodel
`

// Info implements Command.
func (c *exportBundleCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "export-bundle",
		Purpose: "Exports the current model configuration as bundle.",
		Doc:     exportBundleHelpDoc,
	}
}

// SetFlags implements Command.
func (c *exportBundleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.StringVar(&c.Filename, "filename", "", "Bundle file")
}

// Init implements Command.
func (c *exportBundleCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// ExportBundleAPI specifies the used function calls of the BundleFacade.
type ExportBundleAPI interface {
	Close() error
	ExportBundle() (string, error)
}

func (c *exportBundleCommand) getAPI() (ExportBundleAPI, error) {
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}

	return bundle.NewClient(api), nil
}

// Run implements Command.
func (c *exportBundleCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ExportBundle()
	if err != nil {
		return err
	}

	if c.Filename == "" {
		return c.out.Write(ctx, result)
	}
	filename := c.Filename + ".yaml"
	file, err := os.Create(filename)
	if err != nil {
		return errors.Annotate(err, "while creating local file")
	}
	defer file.Close()

	_, err = file.WriteString(result)
	if err != nil {
		return errors.Annotate(err, "while copying in local file")
	}

	fmt.Fprintln(ctx.Stdout, "Bundle successfully exported to", filename)

	return nil
}
