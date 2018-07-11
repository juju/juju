// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package bundle

import (
	"fmt"
	"math/rand"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/api/bundle"
	"github.com/juju/juju/cmd/modelcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.bundle")

// NewexportbundleCommand returns a fully constructed exportbundle command.
func NewExportBundleCommand() cmd.Command {
	return modelcmd.Wrap(&exportBundleCommand{}, modelcmd.WrapSkipModelFlags)
}

type exportBundleCommand struct {
	modelcmd.ModelCommandBase
	api ExportBundleModelAPI
	// name of the charm bundle file.
	Filename string
	modelTag names.ModelTag
}

const exportBundleHelpDoc = `
Exports the current model configuration into a YAML file.

Examples:

    juju export-bundle

If --filename is not used, the archive is downloaded to a temp file
which the name of the model with randnumber appended.
The last exported can be identified manually by user.
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
		return errors.Annotate(err, "retrieving current model configuration details.")
	}
	c.modelTag = names.NewModelTag(modelDetails.ModelUUID)

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
		filename = c.modelTag.String()
		if _, err := os.Stat(filename); err == nil {
			tag := fmt.Sprintf("%v",rand.Intn(1000))
			filename = filename + tag
		}
	}
		return filename
}