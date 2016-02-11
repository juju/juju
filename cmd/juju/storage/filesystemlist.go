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

func newFilesystemListCommand() cmd.Command {
	cmd := &filesystemListCommand{}
	cmd.newAPIFunc = func() (FilesystemListAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

const filesystemListCommandDoc = `
List filesystems in the model.

options:
-m, --model (= "")
    juju model to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

// filesystemListCommand lists storage filesystems.
type filesystemListCommand struct {
	FilesystemCommandBase
	Ids        []string
	out        cmd.Output
	newAPIFunc func() (FilesystemListAPI, error)
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
	c.FilesystemCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFilesystemListTabular,
	})
}

// Run implements Command.Run.
func (c *filesystemListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

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
	ListFilesystems(machines []string) ([]params.FilesystemDetailsListResult, error)
}
