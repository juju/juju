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

func newFilesystemListCommand() cmd.Command {
	return envcmd.Wrap(&filesystemListCommand{})
}

const filesystemListCommandDoc = `
List filesystems in the environment.

options:
-e, --environment (= "")
    juju environment to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

// filesystemListCommand lists storage filesystems.
type filesystemListCommand struct {
	FilesystemCommandBase
	Ids []string
	out cmd.Output
	api FilesystemListAPI
}

// Init implements Command.Init.
func (c *filesystemListCommand) Init(args []string) (err error) {
	c.Ids = args
	return nil
}

// Info implements Command.Info.
func (c *filesystemListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage filesystems",
		Doc:     filesystemListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *filesystemListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFilesystemListTabular,
	})
}

// Run implements Command.Run.
func (c *filesystemListCommand) Run(ctx *cmd.Context) (err error) {
	api := c.api
	if api == nil {
		api, err := c.NewStorageAPI()
		if err != nil {
			return err
		}
		defer api.Close()
	}

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
	info, err := convertToFilesystemInfo(valid)
	if err != nil {
		return err
	}

	var output interface{}
	switch c.out.Name() {
	case "json", "yaml":
		output = map[string]map[string]FilesystemInfo{"filesystems": info}
	default:
		output = info
	}
	return c.out.Write(ctx, output)
}

// FilesystemListAPI defines the API methods that the filesystem list command use.
type FilesystemListAPI interface {
	Close() error
	ListFilesystems(machines []string) ([]params.FilesystemDetailsResult, error)
}
