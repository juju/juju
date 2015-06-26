// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const ListCommandDoc = `
List information about storage instances.

options:
-e, --environment (= "")
   juju environment to operate in
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (json|tabular|yaml)
`

// ListCommand returns storage instances.
type ListCommand struct {
	StorageCommandBase
	out cmd.Output
}

// Init implements Command.Init.
func (c *ListCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "lists storage",
		Doc:     ListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
	})
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getStorageListAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := api.List()
	if err != nil {
		return err
	}
	// filter out valid output, if any
	var valid []params.StorageDetails
	for _, one := range found {
		if one.Error == nil {
			valid = append(valid, one.StorageDetails)
			continue
		}
		// display individual error
		fmt.Fprintf(ctx.Stderr, "%v\n", one.Error)
	}
	if len(valid) == 0 {
		return nil
	}
	output, err := formatStorageDetails(valid)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

var (
	getStorageListAPI = (*ListCommand).getStorageListAPI
)

// StorageAPI defines the API methods that the storage commands use.
type StorageListAPI interface {
	Close() error
	List() ([]params.StorageInfo, error)
}

func (c *ListCommand) getStorageListAPI() (StorageListAPI, error) {
	return c.NewStorageAPI()
}
