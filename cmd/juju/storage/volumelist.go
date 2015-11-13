// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

// VolumeListAPI defines the API methods that the volume list command use.
type VolumeListAPI interface {
	Close() error
	ListVolumes(machines []string) ([]params.VolumeDetailsResult, error)
}

const volumeListCommandDoc = `
List volumes (disks) in the environment.

options:
-e, --environment (= "")
    juju environment to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

func newVolumeListCommand() cmd.Command {
	return envcmd.Wrap(&volumeListCommand{})
}

// volumeListCommand lists storage volumes.
type volumeListCommand struct {
	VolumeCommandBase
	api VolumeListAPI
	Ids []string
	out cmd.Output
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
	c.StorageCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatVolumeListTabular,
	})
}

// Run implements Command.Run.
func (c *volumeListCommand) Run(ctx *cmd.Context) (err error) {
	if c.api == nil {
		api, err := c.NewStorageAPI()
		if err != nil {
			return err
		}
		defer api.Close()
		c.api = api
	}

	found, err := c.api.ListVolumes(c.Ids)
	if err != nil {
		return err
	}
	// filter out valid output, if any
	var valid []params.VolumeDetailsResult
	for _, one := range found {
		if one.Error == nil {
			valid = append(valid, one)
			continue
		}
		// display individual error
		fmt.Fprintf(ctx.Stderr, "%v\n", one.Error)
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
