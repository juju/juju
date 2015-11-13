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

func newListCommand() cmd.Command {
	return envcmd.Wrap(&listCommand{})
}

const listCommandDoc = `
List information about storage instances.

options:
-e, --environment (= "")
   juju environment to operate in
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (json|tabular|yaml)
`

// listCommand returns storage instances.
type listCommand struct {
	StorageCommandBase
	out cmd.Output
	api StorageListAPI
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "lists storage",
		Doc:     listCommandDoc,
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
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api := c.api
	if api == nil {
		api, err = c.NewStorageAPI()
		if err != nil {
			return err
		}
		defer api.Close()
	}

	found, err := api.List()
	if err != nil {
		return err
	}
	// filter out valid output, if any
	var valid []params.StorageDetails
	for _, one := range found {
		if one.Error != nil {
			fmt.Fprintf(ctx.Stderr, "%v\n", one.Error)
			continue
		}
		if one.Result != nil {
			valid = append(valid, *one.Result)
		} else {
			details := storageDetailsFromLegacy(one.Legacy)
			valid = append(valid, details)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	details, err := formatStorageDetails(valid)
	if err != nil {
		return err
	}
	var output interface{}
	switch c.out.Name() {
	case "yaml", "json":
		output = map[string]map[string]StorageInfo{"storage": details}
	default:
		output = details
	}
	return c.out.Write(ctx, output)
}

// StorageAPI defines the API methods that the storage commands use.
type StorageListAPI interface {
	Close() error
	List() ([]params.StorageDetailsResult, error)
}
