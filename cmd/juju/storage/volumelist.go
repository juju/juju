// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// VolumeListAPI defines the API methods that the volume list command use.
type VolumeListAPI interface {
	Close() error
	ListVolumes(machines []string) ([]params.VolumeDetailsListResult, error)
}

const volumeListCommandDoc = `
List volumes (disks) in the model.

options:
-m, --model (= "")
    juju model to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

func newVolumeListCommand() cmd.Command {
	cmd := &volumeListCommand{}
	cmd.newAPIFunc = func() (VolumeListAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// volumeListCommand lists storage volumes.
type volumeListCommand struct {
	VolumeCommandBase
	api        VolumeListAPI
	Ids        []string
	out        cmd.Output
	newAPIFunc func() (VolumeListAPI, error)
}

// Init implements Command.Init.
func (c *volumeListCommand) Init(args []string) (err error) {
	c.Ids = args
	return nil
}

// Info implements Command.Info.
func (c *volumeListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage volumes",
		Doc:     volumeListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *volumeListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.VolumeCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatVolumeListTabular,
	})
}

// Run implements Command.Run.
func (c *volumeListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	results, err := api.ListVolumes(c.Ids)
	if err != nil {
		return err
	}
	// filter out valid output, if any
	var valid []params.VolumeDetails
	for _, result := range results {
		if result.Error == nil {
			valid = append(valid, result.Result...)
			continue
		}
		// display individual error
		fmt.Fprintf(ctx.Stderr, "%v\n", result.Error)
	}
	if len(valid) == 0 {
		return nil
	}

	info, err := convertToVolumeInfo(valid)
	if err != nil {
		return err
	}

	var output interface{}
	switch c.out.Name() {
	case "json", "yaml":
		output = map[string]map[string]VolumeInfo{"volumes": info}
	default:
		output = info
	}
	return c.out.Write(ctx, output)
}
