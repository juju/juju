// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

// StorageListCommand implements the storage-list command.
//
// StorageListCommand implements cmd.Command.
type StorageListCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

func NewStorageListCommand(ctx Context) cmd.Command {
	return &StorageListCommand{ctx: ctx}
}

func (c *StorageListCommand) Info() *cmd.Info {
	doc := `
storage-list will list the names of all storage instances
attached to the unit. These names can be passed to storage-get
via the "-s" flag to query the storage attributes.
`
	return &cmd.Info{
		Name:    "storage-list",
		Purpose: "list storage attached to the unit",
		Doc:     doc,
	}
}

func (c *StorageListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *StorageListCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

func (c *StorageListCommand) Run(ctx *cmd.Context) error {
	tags := c.ctx.StorageTags()
	names := make([]string, len(tags))
	for i, tag := range tags {
		names[i] = tag.Id()
	}
	return c.out.Write(ctx, names)
}
