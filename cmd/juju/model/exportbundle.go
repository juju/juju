// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"os"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/v3/api/client/application"
	"github.com/juju/juju/v3/api/client/bundle"
	jujucmd "github.com/juju/juju/v3/cmd"
	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/rpc/params"
)

// NewExportBundleCommand returns a fully constructed export bundle command.
func NewExportBundleCommand() cmd.Command {
	command := &exportBundleCommand{}
	command.newAPIFunc = func() (ExportBundleAPI, ConfigAPI, error) {
		return command.getAPIs()
	}
	return modelcmd.Wrap(command)
}

type exportBundleCommand struct {
	modelcmd.ModelCommandBase
	out                  cmd.Output
	newAPIFunc           func() (ExportBundleAPI, ConfigAPI, error)
	Filename             string
	includeCharmDefaults bool
}

const exportBundleHelpDoc = `
Exports the current model configuration as a reusable bundle.

If --filename is not used, the configuration is printed to stdout.
 --filename specifies an output file.

Examples:

    juju export-bundle
    juju export-bundle --filename mymodel.yaml
    juju export-bundle --include-charm-defaults

`

// Info implements Command.
func (c *exportBundleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "export-bundle",
		Purpose: "Exports the current model configuration as a reusable bundle.",
		Doc:     exportBundleHelpDoc,
	})
}

// SetFlags implements Command.
func (c *exportBundleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Filename, "filename", "", "Bundle file")
	f.BoolVar(&c.includeCharmDefaults, "include-charm-defaults", false, "Whether to include charm config default values in the exported bundle")
}

// Init implements Command.
func (c *exportBundleCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// ExportBundleAPI specifies the used function calls of the BundleFacade.
type ExportBundleAPI interface {
	Close() error
	ExportBundle(bool) (string, error)
}

// ConfigAPI specifies the used function calls of the ApplicationFacade.
type ConfigAPI interface {
	Close() error
	Get(branchName string, application string) (*params.ApplicationGetResults, error)
}

func (c *exportBundleCommand) getAPIs() (ExportBundleAPI, ConfigAPI, error) {
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, nil, err
	}

	return bundle.NewClient(api), application.NewClient(api), nil
}

// Run implements Command.
func (c *exportBundleCommand) Run(ctx *cmd.Context) error {
	bundleClient, cfgClient, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer func() {
		_ = bundleClient.Close()
		_ = cfgClient.Close()
	}()

	result, err := bundleClient.ExportBundle(c.includeCharmDefaults)
	if err != nil {
		return err
	}

	if c.Filename == "" {
		_, err := fmt.Fprintf(ctx.Stdout, "%v", result)
		return err
	}
	filename := c.Filename
	file, err := c.Filesystem().OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
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
