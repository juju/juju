// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const FilesystemListCommandDoc = `
List filesystems in the environment.

options:
-e, --environment (= "")
    juju environment to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

// FilesystemListCommand lists storage filesystems.
type FilesystemListCommand struct {
	FilesystemCommandBase
	Ids []string
	out cmd.Output
}

// Init implements Command.Init.
func (c *FilesystemListCommand) Init(args []string) (err error) {
	c.Ids = args
	return nil
}

// Info implements Command.Info.
func (c *FilesystemListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage filesystems",
		Doc:     FilesystemListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *FilesystemListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFilesystemListTabular,
	})
}

// Run implements Command.Run.
func (c *FilesystemListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getFilesystemListAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := api.ListFilesystems(c.Ids)
	if err != nil {
		return err
	}
	// filter out valid output, if any
	var valid []params.FilesystemDetailsResult
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
	output, err := convertToFilesystemInfo(valid)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

var getFilesystemListAPI = (*FilesystemListCommand).getFilesystemListAPI

// FilesystemListAPI defines the API methods that the filesystem list command use.
type FilesystemListAPI interface {
	Close() error
	ListFilesystems(machines []string) ([]params.FilesystemDetailsResult, error)
}

func (c *FilesystemListCommand) getFilesystemListAPI() (FilesystemListAPI, error) {
	return c.NewStorageAPI()
}
