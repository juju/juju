// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewListCommand returns a command for listing storage instances.
func NewListCommand() cmd.Command {
	cmd := &listCommand{}
	cmd.newAPIFunc = func() (StorageListAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

const listCommandDoc = `
List information about storage instances.
`

// listCommand returns storage instances.
type listCommand struct {
	StorageCommandBase
	out        cmd.Output
	ids        []string
	filesystem bool
	volume     bool
	newAPIFunc func() (StorageListAPI, error)
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	c.ids = args
	return nil
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "storage",
		Args:    "<machineID> ...",
		Purpose: "Lists storage details.",
		Doc:     listCommandDoc,
		Aliases: []string{"list-storage"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
	})
	f.BoolVar(&c.filesystem, "filesystem", false, "List filesystem storage")
	f.BoolVar(&c.volume, "volume", false, "List volume storage")
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	var output interface{}
	if c.filesystem {
		output, err = c.generateListFilesystemsOutput(ctx, api)
	} else if c.volume {
		output, err = c.generateListVolumeOutput(ctx, api)
	} else {
		output, err = c.generateListOutput(ctx, api)
	}
	if err != nil {
		return err
	}
	if output == nil {
		return nil
	}
	return c.out.Write(ctx, output)
}

// StorageAPI defines the API methods that the storage commands use.
type StorageListAPI interface {
	Close() error
	ListStorageDetails() ([]params.StorageDetails, error)
	ListFilesystems(machines []string) ([]params.FilesystemDetailsListResult, error)
	ListVolumes(machines []string) ([]params.VolumeDetailsListResult, error)
}

// generateListOutput returns a map of storage details
func (c *listCommand) generateListOutput(ctx *cmd.Context, api StorageListAPI) (output interface{}, err error) {

	results, err := api.ListStorageDetails()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	details, err := formatStorageDetails(results)
	if err != nil {
		return nil, err
	}
	switch c.out.Name() {
	case "yaml", "json":
		output = map[string]map[string]StorageInfo{"storage": details}
	default:
		output = details
	}
	return output, nil
}

func formatListTabular(writer io.Writer, value interface{}) error {

	switch value.(type) {
	case map[string]StorageInfo:
		return formatStorageListTabular(writer, value)

	case map[string]FilesystemInfo:
		return formatFilesystemListTabular(writer, value)

	case map[string]VolumeInfo:
		return formatVolumeListTabular(writer, value)

	default:
		return errors.Errorf("unexpected value of type %T", value)
	}
}
