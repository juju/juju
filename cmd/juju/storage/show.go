// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/rpc/params"
)

// NewShowCommand returns a command that shows storage details
// on the specified machine
func NewShowCommand() cmd.Command {
	cmd := &showCommand{}
	cmd.newAPIFunc = func() (StorageShowAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

const showCommandDoc = `
Show extended information about storage instances.

Storage instances to display are specified by storage IDs.

Storage IDs are positional arguments to the command and do not need to be comma
separated when more than one ID is desired.

`

const showCommandExample = `
    juju show-storage storage-id
`

// showCommand attempts to release storage instance.
type showCommand struct {
	StorageCommandBase
	ids        []string
	out        cmd.Output
	newAPIFunc func() (StorageShowAPI, error)
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("must specify storage ID(s)")
	}
	c.ids = args
	return nil
}

// Info implements Command.Info.
func (c *showCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-storage",
		Args:     "<storage ID> [...]",
		Purpose:  "Shows storage instance information.",
		Doc:      showCommandDoc,
		Examples: showCommandExample,
		SeeAlso: []string{
			"storage",
			"attach-storage",
			"detach-storage",
			"remove-storage",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *showCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Run implements Command.Run.
func (c *showCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	tags, err := c.getStorageTags()
	if err != nil {
		return err
	}

	results, err := api.StorageDetails(tags)
	if err != nil {
		return err
	}

	var errs params.ErrorResults
	var valid []params.StorageDetails
	for _, result := range results {
		if result.Error != nil {
			errs.Results = append(errs.Results, params.ErrorResult{result.Error})
			continue
		}
		valid = append(valid, *result.Result)
	}
	if len(errs.Results) > 0 {
		return errs.Combine()
	}

	output, err := formatStorageDetails(valid)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

func (c *showCommand) getStorageTags() ([]names.StorageTag, error) {
	tags := make([]names.StorageTag, len(c.ids))
	for i, id := range c.ids {
		if !names.IsValidStorage(id) {
			return nil, errors.Errorf("invalid storage ID %v", id)
		}
		tags[i] = names.NewStorageTag(id)
	}
	return tags, nil
}

// StorageAPI defines the API methods that the storage commands use.
type StorageShowAPI interface {
	Close() error
	StorageDetails(tags []names.StorageTag) ([]params.StorageDetailsResult, error)
}
