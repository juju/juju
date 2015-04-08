// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const ShowCommandDoc = `
Show extended information about storage instances.
Storage instances to display are specified by storage ids.

* note use of positional arguments

options:
-e, --environment (= "")
   juju environment to operate in
-o, --output (= "")
   specify an output file
--format (= yaml)
   specify output format (json|yaml)
[space separated storage ids]
`

// ShowCommand attempts to release storage instance.
type ShowCommand struct {
	StorageCommandBase
	ids []string
	out cmd.Output
}

// Init implements Command.Init.
func (c *ShowCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("must specify storage id(s)")
	}
	c.ids = args
	return nil
}

// Info implements Command.Info.
func (c *ShowCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show",
		Purpose: "shows storage instance",
		Doc:     ShowCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ShowCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

// Run implements Command.Run.
func (c *ShowCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getStorageShowAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	tags, err := c.getStorageTags()
	if err != nil {
		return err
	}
	found, err := api.Show(tags)
	if err != nil {
		return err
	}
	output, err := formatStorageDetails(found)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

func (c *ShowCommand) getStorageTags() ([]names.StorageTag, error) {
	tags := make([]names.StorageTag, len(c.ids))
	for i, id := range c.ids {
		if !names.IsValidStorage(id) {
			return nil, errors.Errorf("invalid storage id %v", id)
		}
		tags[i] = names.NewStorageTag(id)
	}
	return tags, nil
}

var (
	getStorageShowAPI = (*ShowCommand).getStorageShowAPI
)

// StorageAPI defines the API methods that the storage commands use.
type StorageShowAPI interface {
	Close() error
	Show(tags []names.StorageTag) ([]params.StorageDetails, error)
}

func (c *ShowCommand) getStorageShowAPI() (StorageShowAPI, error) {
	return c.NewStorageAPI()
}
