// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/storage"
)

// StorageGetCommand implements the storage-get command.
type StorageGetCommand struct {
	cmd.CommandBase
	ctx                Context
	storageInstanceIds []string
	out                cmd.Output
}

func NewStorageGetCommand(ctx Context) cmd.Command {
	return &StorageGetCommand{ctx: ctx}
}

func (c *StorageGetCommand) Info() *cmd.Info {
	doc := `
When no <storageInstanceId> is supplied, all storage instances are printed.
`
	return &cmd.Info{
		Name:    "storage-get",
		Args:    "[<storageInstanceId>]*",
		Purpose: "print storage information",
		Doc:     doc,
	}
}

func (c *StorageGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *StorageGetCommand) Init(args []string) error {
	c.storageInstanceIds = args
	return nil
}

func wantInstance(storageId string, storageIds []string) bool {
	for _, id := range storageIds {
		if id == storageId {
			return true
		}
	}
	return false
}

func (c *StorageGetCommand) Run(ctx *cmd.Context) error {
	storageInstances, ok := c.ctx.StorageInstances()
	if !ok {
		return nil
	}
	var value []storage.StorageInstance
	if len(c.storageInstanceIds) > 0 {
		for _, instance := range storageInstances {
			if wantInstance(instance.Id, c.storageInstanceIds) {
				value = append(value, instance)
			}
		}
	} else {
		value = storageInstances
	}
	return c.out.Write(ctx, value)
}
