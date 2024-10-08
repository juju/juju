// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
)

// StorageListCommand implements the storage-list command.
//
// StorageListCommand implements cmd.Command.
type StorageListCommand struct {
	cmd.CommandBase
	ctx         Context
	out         cmd.Output
	storageName string
}

func NewStorageListCommand(ctx Context) (cmd.Command, error) {
	return &StorageListCommand{ctx: ctx}, nil
}

func (c *StorageListCommand) Info() *cmd.Info {
	doc := `
storage-list will list the names of all storage instances
attached to the unit. These names can be passed to storage-get
via the "-s" flag to query the storage attributes.

A storage name may be specified, in which case only storage
instances for that named storage will be returned.

Further details:
storage-list list storages instances that are attached to the unit.
The storage instance identifiers returned from storage-list may be
passed through to the storage-get command using the -s option.
`
	examples := `
    storage-list pgdata
`
	return jujucmd.Info(&cmd.Info{
		Name:     "storage-list",
		Args:     "[<storage-name>]",
		Purpose:  "List storage attached to the unit.",
		Doc:      doc,
		Examples: examples,
	})
}

func (c *StorageListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

func (c *StorageListCommand) Init(args []string) (err error) {
	storageName, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.storageName = storageName
	return nil
}

func (c *StorageListCommand) Run(ctx *cmd.Context) error {
	tags, err := c.ctx.StorageTags(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	ids := make([]string, 0, len(tags))
	for _, tag := range tags {
		id := tag.Id()
		if c.storageName != "" {
			storageName, err := names.StorageName(id)
			if err != nil {
				return errors.Trace(err)
			}
			if storageName != c.storageName {
				continue
			}
		}
		ids = append(ids, id)
	}
	return c.out.Write(ctx, ids)
}
