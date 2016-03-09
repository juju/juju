// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

func newShowCommand() cmd.Command {
	cmd := &showCommand{}
	cmd.newAPIFunc = func() (StorageShowAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

const showCommandDoc = `
Show extended information about storage instances.
Storage instances to display are specified by storage ids.

* note use of positional arguments

options:
-m, --model (= "")
   juju model to operate in
-o, --output (= "")
   specify an output file
--format (= yaml)
   specify output format (json|yaml)
[space separated storage ids]
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
		return errors.New("must specify storage id(s)")
	}
	c.ids = args
	return nil
}

// Info implements Command.Info.
func (c *showCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show",
		Purpose: "shows storage instance",
		Doc:     showCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
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
			return nil, errors.Errorf("invalid storage id %v", id)
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
