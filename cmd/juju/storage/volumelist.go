// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const VolumeListCommandDoc = `
List volumes (disks) in the environment.

options:
-e, --environment (= "")
    juju environment to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

// VolumeListCommand lists storage volumes.
type VolumeListCommand struct {
	VolumeCommandBase
	Ids []string
	out cmd.Output
}

// Init implements Command.Init.
func (c *VolumeListCommand) Init(args []string) (err error) {
	c.Ids = args
	return nil
}

// Info implements Command.Info.
func (c *VolumeListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage volumes",
		Doc:     VolumeListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *VolumeListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatVolumeListTabular,
	})
}

// Run implements Command.Run.
func (c *VolumeListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getVolumeListAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := api.ListVolumes(c.Ids)
	if err != nil {
		return err
	}
	// filter out valid output, if any
	var valid []params.VolumeItem
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
	output, err := convertToVolumeInfo(valid)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

var getVolumeListAPI = (*VolumeListCommand).getVolumeListAPI

// VolumeListAPI defines the API methods that the volume list command use.
type VolumeListAPI interface {
	Close() error
	ListVolumes(machines []string) ([]params.VolumeItem, error)
}

func (c *VolumeListCommand) getVolumeListAPI() (VolumeListAPI, error) {
	return c.NewStorageAPI()
}
