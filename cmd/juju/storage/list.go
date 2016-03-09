// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

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

options:
-m, --model (= "")
   juju model to operate in
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (json|tabular|yaml)
`

// listCommand returns storage instances.
type listCommand struct {
	StorageCommandBase
	out        cmd.Output
	Ids        []string
	filesystem bool
	volume     bool
	newAPIFunc func() (StorageListAPI, error)
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	c.Ids = args
	return nil
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-storage",
		Purpose: "lists storage",
		Doc:     listCommandDoc,
		Aliases: []string{"storage"},
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
	f.BoolVar(&c.filesystem, "filesystem", false, "list filesystem storage")
	f.BoolVar(&c.volume, "volume", false, "list volume storage")
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
		results, err := api.ListFilesystems(c.Ids)
		if err != nil {
			return err
		}
		// filter out valid output, if any
		var valid []params.FilesystemDetails
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
		info, err := convertToFilesystemInfo(valid)
		if err != nil {
			return err
		}
		switch c.out.Name() {
		case "yaml", "json":
			output = map[string]map[string]FilesystemInfo{"filesystems": info}
		default:
			output = info
		}

	} else if c.volume {
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
		switch c.out.Name() {
		case "yaml", "json":
			output = map[string]map[string]VolumeInfo{"volumes": info}
		default:
			output = info
		}
	} else {

		results, err := api.ListStorageDetails()
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return nil
		}
		details, err := formatStorageDetails(results)
		if err != nil {
			return err
		}
		switch c.out.Name() {
		case "yaml", "json":
			output = map[string]map[string]StorageInfo{"storage": details}
		default:
			output = details
		}
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

func formatListTabular(value interface{}) ([]byte, error) {
	_, ok := value.(map[string]StorageInfo)
	if ok {
		output, err := formatStorageListTabular(value)
		return output, err
	}
	_, ok2 := value.(map[string]FilesystemInfo)
	if ok2 {
		output, err := formatFilesystemListTabular(value)
		return output, err
	}
	_, ok3 := value.(map[string]VolumeInfo)
	if ok3 {
		output, err := formatVolumeListTabular(value)
		return output, err
	} else {
		return nil, errors.Errorf("unexpected value of type %T", value)
	}

}
