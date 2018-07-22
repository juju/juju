// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/bundle"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewexportbundleCommand returns a fully constructed exportbundle command.
func NewExportBundleCommand() cmd.Command {
	return modelcmd.Wrap(&exportBundleCommand{})
}

type exportBundleCommand struct {
	modelcmd.ModelCommandBase
	api ExportBundleModelAPI
	// name of the charm bundle file.
	Filename string
}

const exportBundleHelpDoc = `
Exports the current model configuration into a YAML file.

Examples:

    juju export-bundle

If --filename is not used, the archive is downloaded to a temp file
which the name of the model with time appended.
`

// Info implements Command.
func (c *exportBundleCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "export-bundle",
		Purpose: "Exports the current model configuration in a charm bundle.",
		Doc:     exportBundleHelpDoc,
	}
}

// SetFlags implements Command.
func (c *exportBundleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Filename, "filename", "", "Export Model")
}

// Init implements Command.
func (c *exportBundleCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// ExportBundleAPI specifies the used function calls of the ModelManager.
type ExportBundleModelAPI interface {
	Close() error
	ExportBundle() (string, error)
}

func (c *exportBundleCommand) getAPI() (ExportBundleModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}

	api, err := c.NewAPIRoot()
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

	result, err := client.ExportBundle()
	if err != nil {
		return err
	}

	filename := c.ResolveFilename()
	File, err := os.Create(filename)
	if err != nil {
		return errors.Annotate(err, "while creating local file")
	}
	defer File.Close()

	// Write out the result.
	_, err = File.WriteString(result)
	if err != nil {
		return errors.Annotate(err, "while copying in local file")
	}

	// Print the local filename.
	fmt.Fprintln(ctx.Stdout, filename)
	return nil
}

// ResolveFilename returns the filename used by the command.
func (c *exportBundleCommand) ResolveFilename() string {
	filename := c.Filename
	if filename == "" {
		_, modelDetails, err := c.ModelCommandBase.ModelDetails()
		if err != nil {
			errors.Annotate(err, "retrieving current model configuration details.")
			return ""
		}
		modelTag := names.NewModelTag(modelDetails.ModelUUID)

		filename = modelTag.String()
		if _, err := os.Stat(filename); err == nil {
			currentTime := time.Now().Format(time.RFC822)
			filename = filename + currentTime
		}
	}
	return filename + ".yaml"
}
